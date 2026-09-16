# Counter API V2 Reference

V2 is the stricter API contract for new clients. Existing unversioned routes
remain V1 and continue to support current clients.

> V2 routes are introduced incrementally. An endpoint is available only after
> its implementation has been deployed.

## Contract Guarantees

- Every mutation requires an `Idempotency-Key` header containing a UUID.
- The key is scoped to the tenant and identifies one logical mutation.
- A retry with the same key and the same canonical request returns the original
  result without applying the mutation again.
- Reusing a key with different tenant, counter, operation, or delta data returns
  `409 IDEMPOTENCY_KEY_REUSED`.
- A successful mutation means PostgreSQL committed the counter update and its
  immutable operation record.
- Mutation responses identify whether the result was replayed.

## Authentication

V2 preserves the existing authentication requirements unless an endpoint's V2
contract explicitly documents a change. Existing admin endpoints require the
`X-API-Key` header.

## Increment Counter

Increments a counter by a positive delta.

### Request

```http
POST /v2/tenants/{tenant_id}/counters/{counter_id}/inc?delta=5
Idempotency-Key: 01912345-6789-7000-8000-000000000003
Content-Type: application/json
```

`delta` defaults to `1` and must not exceed the counter's configured
`max_delta`.

### Response

```http
200 OK
Content-Type: application/json

{
  "operation_id": "01912345-6789-7000-8000-000000000003",
  "counter_id": "01912345-6789-7000-8000-000000000002",
  "delta": 5,
  "value": 47,
  "replayed": false,
  "updated_at": "2026-04-07T12:02:00Z"
}
```

An exact retry returns the same result with `replayed: true`.

### Errors

| Status | Code | Meaning |
|--------|------|---------|
| `400` | `IDEMPOTENCY_KEY_REQUIRED` | The request has no `Idempotency-Key` header |
| `400` | `INVALID_IDEMPOTENCY_KEY` | The key is not a valid UUID |
| `400` | `INVALID_DELTA` | Delta is missing, non-positive, or malformed |
| `400` | `DELTA_EXCEEDS_MAXIMUM` | Delta exceeds the counter's configured maximum |
| `404` | `COUNTER_NOT_FOUND` | Counter does not exist for the tenant |
| `409` | `IDEMPOTENCY_KEY_REUSED` | Key was used for different request data |
| `503` | `SERVICE_UNAVAILABLE` | Mutation could not reach a usable database |

## Set Counter Value

The V2 set operation uses the same mandatory idempotency key and durable
operation contract. It records an audited adjustment from the current value to
the requested value; it does not bypass the mutation ledger.

```http
POST /v2/tenants/{tenant_id}/counters/{counter_id}/set
X-API-Key: your-api-key
Idempotency-Key: 01912345-6789-7000-8000-000000000004
Content-Type: application/json

{
  "value": 100
}
```

## Reads and Consistency

V2 reads are strong while served by the PostgreSQL primary. Future stale-capable
or striped reads must expose their freshness/version metadata explicitly; they
must not be presented as exact immediate balances.
