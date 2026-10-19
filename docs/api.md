# Counter API Documentation

All tenant, counter, and credential APIs use `/v2` and require an `X-API-Key`.
Unversioned V1 data routes are retired and return `404` without redirects.
See [the V2 reference](api-v2.md) for request/response examples and the
[V1 migration map](api-v2.md#migrating-from-v1) for old-to-new URLs.

## Authorization

Tenant credentials must match the tenant in the URL and have the action's
scope. Administrator keys may access all tenants. Missing/invalid credentials
return `401`; insufficient scopes or another tenant's credential return `403`.

| Operation | Route | Required tenant scope |
|---|---|---|
| Create tenant | `POST /v2/tenants` | Administrator only |
| Read tenant | `GET /v2/tenants/{tenant_id}` | `tenant:read` |
| Create counter | `POST /v2/tenants/{tenant_id}/counters` | `counter:create` |
| List counters | `GET /v2/tenants/{tenant_id}/counters` | `counter:list` |
| Read counter | `GET /v2/tenants/{tenant_id}/counters/{counter_id}` | `counter:read` |
| Increment | `POST /v2/tenants/{tenant_id}/counters/{counter_id}/inc` | `counter:increment` |
| Set value | `POST /v2/tenants/{tenant_id}/counters/{counter_id}/set` | `counter:adjust` |
| Read history | `GET /v2/tenants/{tenant_id}/counters/{counter_id}/operations` | `counter:history` |
| Read metadata | `GET /v2/tenants/{tenant_id}/counters/{counter_id}/metadata` | `metadata:read` |
| Create/replace metadata | `PUT /v2/tenants/{tenant_id}/counters/{counter_id}/metadata` | `metadata:write` |
| Delete metadata | `DELETE /v2/tenants/{tenant_id}/counters/{counter_id}/metadata` | `metadata:write` |
| Create tenant credential | `POST /v2/tenants/{tenant_id}/credentials` | Administrator only |
| Rotate credential | `POST /v2/tenants/{tenant_id}/credentials/{credential_id}/rotate` | Administrator only |
| Revoke credential | `POST /v2/tenants/{tenant_id}/credentials/{credential_id}/revoke` | Administrator only |
| Create admin credential | `POST /v2/admin/credentials` | Administrator only |

The environment administrator key is accepted when `LEGACY_API_KEY_ENABLED`
is enabled. It is independent of the retired API version. Default tenant
credential scopes are `counter:read`, `counter:increment`, and
`counter:history`; grant other scopes explicitly.

## Idempotency coverage

Generate a UUID for each logical write and reuse it for retries. Support is
optional on tenant creation, counter creation, metadata writes, and all credential lifecycle
routes. Omitting the header preserves existing behavior. Counter increment/set
require the header.

| Routes | `Idempotency-Key` behavior |
|---|---|
| `POST /v2/tenants` | Optional; replay the original tenant response. |
| `POST /v2/tenants/{tenant_id}/counters` | Optional; replay the original counter response. |
| Counter `/inc` and `/set` | Required; use existing durable operation-history replay. |
| Metadata `PUT` and `DELETE` | Optional; replay the original response and ETag. Preconditions are part of the request identity. |
| `POST /v2/tenants/{tenant_id}/credentials` | Optional; replay credential fields with `api_key` omitted. |
| `POST /v2/admin/credentials` | Optional; replay credential fields with `api_key` omitted. |
| `POST /v2/tenants/{tenant_id}/credentials/{credential_id}/rotate` | Optional; replay replacement credential fields with `api_key` omitted. |
| `POST /v2/tenants/{tenant_id}/credentials/{credential_id}/revoke` | Optional; replay the original `204`, even after revocation. |
| All `GET` routes and `OPTIONS` | Ignore the header; do not reserve keys or cache responses for replay. |

For creation, metadata, and credential routes:

- Keys must be UUIDs and are scoped to the authenticated credential, HTTP
  method, and target path. Use the same credential and URL for retries.
- Successful responses are retained for 24 hours. Reusing a key after expiry
  executes a new request; do not rely on deduplication beyond that window.
- Same key and request: return the original status and response, with
  `Idempotency-Replayed: true`. The first successful response has
  `Idempotency-Replayed: false`. Browser clients can read this response header.
- Same key with different body or query parameters: return
  `409 IDEMPOTENCY_KEY_REUSED`. JSON whitespace, unambiguous object property
  order, and query parameter order do not change the request hash. Bodies with
  duplicate or case-equivalent keys retain their original key order because
  reordering those keys can change the decoded write. Explicit defaults or
  other body changes can change the hash.
- Invalid keys return `400 INVALID_IDEMPOTENCY_KEY`. Validation and other
  failed requests do not reserve keys, so a corrected request can reuse one.
- Authentication and authorization run again on every retry. A revoked caller
  cannot retrieve a saved response. Concurrent retries are coordinated through
  PostgreSQL across replicas. A lock timeout or unavailable storage returns
  `503`; retry with the same key.

Credential secrets are returned only on the first successful execution and
are never stored for replay. If that response is lost, a retry identifies the
issued credential but cannot recover its secret. Revoke that credential and
issue another with a new idempotency key. A replay does not reactivate an
expired or revoked credential; it reports the original issuance result.

Counter increment/set keep their existing tenant-wide key scope, `replayed`
response field, and operation-history retention. They do not use the new
24-hour replay table or `Idempotency-Replayed` header.

## Optional counter metadata

Each counter can have one independent JSON document in a separate table. Accepts
objects, arrays, scalars, and JSON `null`, up to 64 KiB and 64 nesting levels,
subject to PostgreSQL JSONB representation limits. JSON `null` is a stored
document; deleting the document removes it. Documents cannot be shared between
counters. Count reads and listings keep their existing response format and do
not fetch metadata.

Use the `/metadata` routes with explicit `metadata:read` or `metadata:write`
scopes; existing/default counter scopes do not grant metadata access. GET
returns a document envelope and a quoted opaque `ETag`. Send the raw JSON value
to PUT with `If-None-Match: *` to create, or `If-Match: <ETag>` to replace.
DELETE requires `If-Match: <ETag>`. Missing conditions return `428`, malformed
conditions `400`, and stale conditions `412`. Obtain a fresh ETag after `412`.
Metadata responses use `Cache-Control: no-store`; browsers can read `ETag`.
See [metadata examples](api-v2.md#counter-metadata).

## Operations and Health

`GET /livez` checks only whether the process is alive and does not require
PostgreSQL. `GET /readyz` returns `200` only after startup has completed, the
database schema matches the binary, the process is not draining, and a
bounded PostgreSQL ping succeeds. Load balancers should use `/readyz` for
traffic eligibility and `/livez` for process supervision.

`GET /metrics` is administrator-protected and returns Prometheus text
including route/status request counts and durations, database pool usage and
waits, Go goroutine/heap/GC snapshots, overload/rate-limit/idempotency events,
build version, and schema version. It must not be exposed directly to the public
internet.


## Requests and tools

Use `Content-Type: application/json` for JSON bodies. The server returns an
`X-Request-ID` response header. Counter values are signed 64-bit integers;
use a JSON implementation that preserves integer precision.

The [HTML reference](api.html) is generated from the Bruno collection. The
browser-only command builder is available at `/tools/curl`. The load-testing
client in `scripts/scalability_load.py` uses authenticated V2 routes.
