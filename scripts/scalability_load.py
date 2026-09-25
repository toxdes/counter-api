#!/usr/bin/env python3
"""Run a reproducible load test against an isolated Counter API deployment."""

from __future__ import annotations

import argparse
import concurrent.futures
import http.client
import ipaddress
import itertools
import json
import math
import os
import platform
import random
import shlex
import statistics
import sys
import threading
import time
import uuid
from dataclasses import asdict, dataclass
from datetime import datetime, timezone
from pathlib import Path
from typing import Any, Iterator
from urllib.parse import quote, urlencode, urlsplit


class CorrectnessError(RuntimeError):
    """Raised when stored state differs from the completed operation history."""


@dataclass(frozen=True)
class CounterTarget:
    tenant_id: str
    counter_id: str


@dataclass(frozen=True)
class WorkItem:
    target_index: int
    operation_id: str
    kind: str


@dataclass
class RequestSample:
    operation_id: str | None
    counter_id: str
    kind: str
    status: int
    latency_ms: float
    replayed: bool = False
    error: str | None = None


def percentile(values: list[float], quantile: float) -> float:
    """Return the nearest-rank percentile (quantile is between 0 and 1)."""
    if not values:
        return 0.0
    if not 0 <= quantile <= 1:
        raise ValueError("quantile must be between 0 and 1")
    ordered = sorted(values)
    return ordered[max(0, math.ceil(quantile * len(ordered)) - 1)]


def verify_counter_history(
    initial_value: int,
    current_value: int,
    operations: list[dict[str, Any]],
    expected_increment_ids: set[str],
) -> dict[str, int]:
    """Check initial record, requested operations, arithmetic, and materialized value."""
    initial_operations = [operation for operation in operations if operation.get("kind") == "initial_value"]
    increments = [operation for operation in operations if operation.get("kind") == "increment"]
    if len(initial_operations) != 1:
        raise CorrectnessError(f"expected one initial_value operation, found {len(initial_operations)}")
    if initial_operations[0].get("value_after") != initial_value:
        raise CorrectnessError("initial_value history does not match the benchmark's configured initial value")

    operation_ids = [str(operation.get("operation_id", "")) for operation in increments]
    if len(operation_ids) != len(set(operation_ids)):
        raise CorrectnessError("duplicate increment operation IDs found in history")
    actual_ids = set(operation_ids)
    if actual_ids != expected_increment_ids:
        missing = sorted(expected_increment_ids - actual_ids)
        unexpected = sorted(actual_ids - expected_increment_ids)
        raise CorrectnessError(f"history operation IDs differ (missing={missing[:5]}, unexpected={unexpected[:5]})")

    for operation in increments:
        before = operation.get("value_before")
        after = operation.get("value_after")
        delta = operation.get("delta")
        if not all(isinstance(value, int) for value in (before, after, delta)):
            raise CorrectnessError(f"operation {operation.get('operation_id')} has incomplete value data")
        if after != before + delta:
            raise CorrectnessError(f"operation {operation.get('operation_id')} has inconsistent delta arithmetic")

    computed_value = initial_value + sum(int(operation["delta"]) for operation in increments)
    if computed_value != current_value:
        raise CorrectnessError(f"history computes {computed_value}, but current value is {current_value}")
    return {"increment_count": len(increments), "computed_value": computed_value}


class APIClient:
    """Small persistent HTTP client with one connection per worker thread."""

    def __init__(self, base_url: str, api_key: str, timeout: float):
        parsed = urlsplit(base_url)
        if parsed.scheme not in {"http", "https"} or not parsed.hostname:
            raise ValueError("base URL must be an http(s) URL with a hostname")
        if parsed.username or parsed.password or parsed.query or parsed.fragment:
            raise ValueError("base URL must not contain credentials, a query, or a fragment")
        self.scheme = parsed.scheme
        self.host = parsed.hostname
        self.port = parsed.port
        self.base_path = parsed.path.rstrip("/")
        self.api_key = api_key
        self.timeout = timeout
        self.connections = threading.local()

    def request(self, method: str, path: str, body: dict[str, Any] | None = None, headers: dict[str, str] | None = None) -> tuple[int, bytes, float]:
        connection = getattr(self.connections, "connection", None)
        if connection is None:
            connection_type = http.client.HTTPSConnection if self.scheme == "https" else http.client.HTTPConnection
            connection = connection_type(self.host, self.port, timeout=self.timeout)
            self.connections.connection = connection

        request_headers = {"X-API-Key": self.api_key, "Accept": "application/json"}
        if body is not None:
            request_headers["Content-Type"] = "application/json"
        if headers:
            request_headers.update(headers)
        payload = json.dumps(body).encode("utf-8") if body is not None else None
        request_path = f"{self.base_path}{path}"
        started = time.perf_counter()
        try:
            connection.request(method, request_path, body=payload, headers=request_headers)
            response = connection.getresponse()
            response_body = response.read()
            elapsed_ms = (time.perf_counter() - started) * 1000
            return response.status, response_body, elapsed_ms
        except (OSError, http.client.HTTPException, TimeoutError) as error:
            connection.close()
            self.connections.connection = None
            elapsed_ms = (time.perf_counter() - started) * 1000
            return 0, str(error).encode("utf-8"), elapsed_ms

    def close_thread_connection(self) -> None:
        connection = getattr(self.connections, "connection", None)
        if connection is not None:
            connection.close()
            self.connections.connection = None


def decode_json(status: int, body: bytes, action: str) -> dict[str, Any]:
    try:
        result = json.loads(body)
    except (UnicodeDecodeError, json.JSONDecodeError) as error:
        details = body.decode("utf-8", "replace")[:300]
        raise RuntimeError(f"{action} returned HTTP {status} with invalid JSON: {details}") from error
    if not 200 <= status < 300:
        raise RuntimeError(f"{action} returned HTTP {status}: {result}")
    if not isinstance(result, dict):
        raise RuntimeError(f"{action} returned a non-object JSON response")
    return result


def create_targets(client: APIClient, tenants: int, counters_per_tenant: int, run_id: str) -> tuple[list[str], list[CounterTarget]]:
    tenant_ids: list[str] = []
    targets: list[CounterTarget] = []
    for tenant_index in range(tenants):
        status, body, _ = client.request("POST", "/tenants", {"label": f"load-{run_id}-{tenant_index}"})
        tenant_id = str(decode_json(status, body, "create tenant").get("tenant_id", ""))
        if not tenant_id:
            raise RuntimeError("create tenant response did not include tenant_id")
        tenant_ids.append(tenant_id)
        for counter_index in range(counters_per_tenant):
            counter_body = {
                "label": f"load-{run_id}-{tenant_index}-{counter_index}",
                "initial_value": 0,
                "max_delta": 1,
            }
            path = f"/tenants/{quote(tenant_id, safe='')}/counters"
            status, body, _ = client.request("POST", path, counter_body)
            counter_id = str(decode_json(status, body, "create counter").get("counter_id", ""))
            if not counter_id:
                raise RuntimeError("create counter response did not include counter_id")
            targets.append(CounterTarget(tenant_id=tenant_id, counter_id=counter_id))
    return tenant_ids, targets


def choose_work_kind(profile: str, rng: random.Random) -> str:
    if profile == "read-heavy" and rng.random() < 0.8:
        return "read"
    return "increment"


def choose_target_index(profile: str, rng: random.Random, tenants: int, counters_per_tenant: int) -> int:
    if profile == "hot-counter":
        return 0
    if profile == "hot-tenant":
        return rng.randrange(counters_per_tenant)
    return rng.randrange(tenants * counters_per_tenant)


def create_work_items(request_count: int, profile: str, seed: int, tenants: int, counters_per_tenant: int) -> Iterator[WorkItem]:
    rng = random.Random(seed)
    for _ in range(request_count):
        yield WorkItem(
            target_index=choose_target_index(profile, rng, tenants, counters_per_tenant),
            operation_id=str(uuid.uuid4()),
            kind=choose_work_kind(profile, rng),
        )


def execute_work_item(client: APIClient, item: WorkItem, targets: list[CounterTarget], profile: str) -> list[RequestSample]:
    target = targets[item.target_index]
    tenant = quote(target.tenant_id, safe="")
    counter = quote(target.counter_id, safe="")
    if item.kind == "read":
        path = f"/tenants/{tenant}/counters/{counter}"
        status, body, elapsed = client.request("GET", path)
        return [RequestSample(None, target.counter_id, "read", status, elapsed, error=None if 200 <= status < 300 else body.decode("utf-8", "replace")[:500])]

    path = f"/v2/tenants/{tenant}/counters/{counter}/inc?{urlencode({'delta': 1})}"
    headers = {"Idempotency-Key": item.operation_id}
    attempt_count = 2 if profile == "retry-heavy" else 1
    samples: list[RequestSample] = []
    for _ in range(attempt_count):
        status, body, elapsed = client.request("POST", path, headers=headers)
        replayed = False
        error = None
        if 200 <= status < 300:
            try:
                replayed = bool(json.loads(body).get("replayed", False))
            except (UnicodeDecodeError, json.JSONDecodeError, AttributeError):
                error = "successful mutation returned malformed JSON"
                status = 0
        else:
            error = body.decode("utf-8", "replace")[:500]
        samples.append(RequestSample(item.operation_id, target.counter_id, "increment", status, elapsed, replayed, error))
    return samples


def fetch_current_value(client: APIClient, target: CounterTarget) -> int:
    path = f"/tenants/{quote(target.tenant_id, safe='')}/counters/{quote(target.counter_id, safe='')}"
    status, body, _ = client.request("GET", path)
    return int(decode_json(status, body, "read counter value")["value"])


def fetch_operations(client: APIClient, target: CounterTarget) -> list[dict[str, Any]]:
    path = f"/v2/tenants/{quote(target.tenant_id, safe='')}/counters/{quote(target.counter_id, safe='')}/operations"
    operations: list[dict[str, Any]] = []
    cursor: str | None = None
    while True:
        query = {"limit": 100}
        if cursor:
            query["cursor"] = cursor
        status, body, _ = client.request("GET", f"{path}?{urlencode(query)}")
        response = decode_json(status, body, "read operation history")
        page = response.get("operations")
        if not isinstance(page, list):
            raise RuntimeError("operation history response did not include an operations array")
        operations.extend(page)
        cursor_value = response.get("next_cursor")
        cursor = str(cursor_value) if cursor_value else None
        if not cursor:
            return operations


def summarize_samples(samples: list[RequestSample], elapsed_seconds: float) -> dict[str, Any]:
    latencies = [sample.latency_ms for sample in samples]
    status_counts: dict[str, int] = {}
    for sample in samples:
        status_counts[str(sample.status)] = status_counts.get(str(sample.status), 0) + 1
    successful = sum(1 for sample in samples if 200 <= sample.status < 300)
    submitted_increment_ids = {sample.operation_id for sample in samples if sample.kind == "increment" and sample.operation_id}
    successful_increment_ids = {
        sample.operation_id
        for sample in samples
        if sample.kind == "increment" and sample.operation_id and 200 <= sample.status < 300
    }
    by_kind: dict[str, list[RequestSample]] = {}
    for sample in samples:
        by_kind.setdefault(sample.kind, []).append(sample)

    def summarize_group(group: list[RequestSample]) -> dict[str, Any]:
        group_latencies = [sample.latency_ms for sample in group]
        group_successful = sum(1 for sample in group if 200 <= sample.status < 300)
        return {
            "requests": len(group),
            "successful_requests": group_successful,
            "failed_requests": len(group) - group_successful,
            "latency_ms": {
                "p50": percentile(group_latencies, 0.50),
                "p95": percentile(group_latencies, 0.95),
                "p99": percentile(group_latencies, 0.99),
            },
        }

    return {
        "http_requests": len(samples),
        "successful_requests": successful,
        "failed_requests": len(samples) - successful,
        "increment_operation_keys_submitted": len(submitted_increment_ids),
        "increment_operation_keys_with_successful_response": len(successful_increment_ids),
        "idempotency_replay_responses": sum(1 for sample in samples if sample.replayed),
        "status_counts": status_counts,
        "throughput_requests_per_second": len(samples) / elapsed_seconds if elapsed_seconds > 0 else 0.0,
        "latency_ms": {
            "min": min(latencies, default=0.0),
            "mean": statistics.fmean(latencies) if latencies else 0.0,
            "p50": percentile(latencies, 0.50),
            "p95": percentile(latencies, 0.95),
            "p99": percentile(latencies, 0.99),
            "max": max(latencies, default=0.0),
        },
        "by_kind": {kind: summarize_group(group) for kind, group in sorted(by_kind.items())},
        "error_examples": [
            {"kind": sample.kind, "status": sample.status, "message": sample.error}
            for sample in samples
            if sample.status < 200 or sample.status >= 300
        ][:20],
    }


def recorded_command(arguments: list[str]) -> str:
    """Preserve the reproducible invocation while hiding an inline API key."""
    sanitized: list[str] = []
    redact_next = False
    for argument in arguments:
        if redact_next:
            sanitized.append("<redacted>")
            redact_next = False
        elif argument == "--api-key":
            sanitized.append(argument)
            redact_next = True
        elif argument.startswith("--api-key="):
            sanitized.append("--api-key=<redacted>")
        else:
            sanitized.append(argument)
    return shlex.join(sanitized)


def is_loopback_target(base_url: str) -> bool:
    hostname = urlsplit(base_url).hostname
    if not hostname:
        return False
    if hostname.lower() == "localhost" or hostname.lower().endswith(".localhost"):
        return True
    try:
        return ipaddress.ip_address(hostname).is_loopback
    except ValueError:
        return False


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--base-url", required=True, help="API base URL, preferably the test load balancer")
    parser.add_argument("--allow-remote-target", action="store_true", help="Confirm that a non-loopback test target may receive benchmark writes")
    parser.add_argument("--api-key", default=os.environ.get("API_KEY"), help="Administrator API key; defaults to API_KEY")
    parser.add_argument("--profile", choices=("distributed", "hot-tenant", "hot-counter", "read-heavy", "retry-heavy"), default="distributed")
    parser.add_argument("--tenants", type=int, default=10)
    parser.add_argument("--counters-per-tenant", type=int, default=10)
    parser.add_argument("--requests", type=int, default=1000, help="Logical requests; retry-heavy sends two HTTP attempts each")
    parser.add_argument("--workers", type=int, default=16)
    parser.add_argument("--seed", type=int, default=1, help="Reproducible target selection and read/write mix")
    parser.add_argument("--timeout-seconds", type=float, default=5.0)
    parser.add_argument("--report", type=Path, help="JSON report path; defaults to a timestamped file in the current directory")
    parser.add_argument("--duration-seconds", type=float, default=0.0, help="Run continuously for this long instead of using --requests")
    parser.add_argument("--metrics-url", action="append", default=[], help="Base URL of a private API replica to scrape at /metrics; repeat per replica")
    parser.add_argument("--metrics-interval-seconds", type=float, default=5.0)
    parser.add_argument("--environment-json", type=Path, help="Optional JSON file with hardware, PostgreSQL, and deployment settings")
    args = parser.parse_args()
    if not args.api_key:
        parser.error("provide --api-key or set API_KEY")
    if (not is_loopback_target(args.base_url) or any(not is_loopback_target(url) for url in args.metrics_url)) and not args.allow_remote_target:
        parser.error("non-loopback API or metrics targets require --allow-remote-target")
    if args.tenants < 1 or args.counters_per_tenant < 1 or args.workers < 1 or (args.duration_seconds == 0 and args.requests < 1):
        parser.error("tenants, counters-per-tenant, requests, and workers must be positive")
    if args.duration_seconds < 0 or args.timeout_seconds <= 0 or args.metrics_interval_seconds <= 0:
        parser.error("duration cannot be negative; timeouts and metrics interval must be positive")
    return args


def main() -> int:
    args = parse_args()
    run_id = datetime.now(timezone.utc).strftime("%Y%m%dT%H%M%SZ") + "-" + uuid.uuid4().hex[:8]
    base_client = APIClient(args.base_url, args.api_key, args.timeout_seconds)
    tenant_ids, targets = create_targets(base_client, args.tenants, args.counters_per_tenant, run_id)
    started_at = datetime.now(timezone.utc).isoformat()
    samples: list[RequestSample] = []
    started = time.perf_counter()

    def run_item(item: WorkItem) -> list[RequestSample]:
        return execute_work_item(base_client, item, targets, args.profile)

    metric_stop = threading.Event()
    metrics_samples: list[dict[str, Any]] = []

    def sample_metrics() -> None:
        clients = [(url, APIClient(url, args.api_key, args.timeout_seconds)) for url in args.metrics_url]
        try:
            while not metric_stop.is_set():
                for url, client in clients:
                    status, body, latency = client.request("GET", "/metrics")
                    metrics_samples.append({
                        "sampled_at": datetime.now(timezone.utc).isoformat(),
                        "url": url,
                        "status": status,
                        "latency_ms": latency,
                        "body": body.decode("utf-8", "replace")[:200_000],
                    })
                if metric_stop.wait(args.metrics_interval_seconds):
                    break
        finally:
            for _, client in clients:
                client.close_thread_connection()

    metrics_thread = None
    if args.metrics_url:
        metrics_thread = threading.Thread(target=sample_metrics, name="metrics-scraper", daemon=True)
        metrics_thread.start()

    # Process bounded batches to avoid retaining one Future per request.
    batch_size = max(args.workers * 8, 64)
    try:
        with concurrent.futures.ThreadPoolExecutor(max_workers=args.workers) as executor:
            if args.duration_seconds > 0:
                deadline = time.perf_counter() + args.duration_seconds

                def soak_worker(worker_index: int) -> list[RequestSample]:
                    rng = random.Random(args.seed + worker_index)
                    worker_samples: list[RequestSample] = []
                    while time.perf_counter() < deadline:
                        item = WorkItem(
                            target_index=choose_target_index(args.profile, rng, args.tenants, args.counters_per_tenant),
                            operation_id=str(uuid.uuid4()),
                            kind=choose_work_kind(args.profile, rng),
                        )
                        worker_samples.extend(run_item(item))
                    return worker_samples

                for worker_samples in executor.map(soak_worker, range(args.workers)):
                    samples.extend(worker_samples)
            else:
                work_items = create_work_items(args.requests, args.profile, args.seed, args.tenants, args.counters_per_tenant)
                while batch := list(itertools.islice(work_items, batch_size)):
                    for result in executor.map(run_item, batch):
                        samples.extend(result)
    finally:
        metric_stop.set()
        if metrics_thread is not None:
            metrics_thread.join(timeout=args.timeout_seconds * max(1, len(args.metrics_url)) + 1)
    elapsed_seconds = time.perf_counter() - started

    samples_by_counter: dict[str, list[RequestSample]] = {}
    for sample in samples:
        if sample.kind == "increment" and sample.operation_id:
            samples_by_counter.setdefault(sample.counter_id, []).append(sample)

    correctness: list[dict[str, Any]] = []
    all_correct = True
    for target in targets:
        operations = fetch_operations(base_client, target)
        current_value = fetch_current_value(base_client, target)
        target_samples = samples_by_counter.get(target.counter_id, [])
        submitted_ids = {sample.operation_id for sample in target_samples if sample.operation_id}
        successful_ids = {sample.operation_id for sample in target_samples if sample.operation_id and 200 <= sample.status < 300}
        observed_ids = {str(operation.get("operation_id")) for operation in operations if operation.get("kind") == "increment"}
        expected_ids = successful_ids | (submitted_ids & observed_ids)
        try:
            result = verify_counter_history(0, current_value, operations, expected_ids)
            correctness.append({"tenant_id": target.tenant_id, "counter_id": target.counter_id, "ok": True, **result})
        except CorrectnessError as error:
            all_correct = False
            correctness.append({"tenant_id": target.tenant_id, "counter_id": target.counter_id, "ok": False, "error": str(error)})

    environment: dict[str, Any] = {}
    if args.environment_json:
        environment = json.loads(args.environment_json.read_text(encoding="utf-8"))
        if not isinstance(environment, dict):
            raise RuntimeError("environment JSON must contain an object")
    report_path = args.report or Path(f"scalability-report-{run_id}.json")
    accepted_increments = sum(result.get("increment_count", 0) for result in correctness)
    summary = summarize_samples(samples, elapsed_seconds)
    summary["accepted_increment_operations_per_second"] = accepted_increments / elapsed_seconds if elapsed_seconds > 0 else 0.0
    report = {
        "run_id": run_id,
        "started_at": started_at,
        "command": recorded_command(sys.argv),
        "python_version": platform.python_version(),
        "client_platform": platform.platform(),
        "configuration": {
            "base_url": args.base_url,
            "remote_target_confirmed": args.allow_remote_target,
            "profile": args.profile,
            "tenants": args.tenants,
            "counters_per_tenant": args.counters_per_tenant,
            "logical_requests": args.requests if args.duration_seconds == 0 else None,
            "duration_seconds": args.duration_seconds or None,
            "workers": args.workers,
            "seed": args.seed,
            "timeout_seconds": args.timeout_seconds,
            "tenant_ids": tenant_ids,
            "targets": [asdict(target) for target in targets],
        },
        "environment": environment,
        "elapsed_seconds": elapsed_seconds,
        "summary": summary,
        "metrics_samples": metrics_samples,
        "correctness": {
            "ok": all_correct,
            "counters_checked": len(correctness),
            "increments_in_history": accepted_increments,
            "counters": correctness,
        },
    }
    report_path.write_text(json.dumps(report, indent=2, sort_keys=True) + "\n", encoding="utf-8")
    base_client.close_thread_connection()
    print(json.dumps({"report": str(report_path), "summary": report["summary"], "correctness": report["correctness"]}, indent=2))
    return 0 if all_correct else 2


if __name__ == "__main__":
    raise SystemExit(main())
