# Counter API V2 Reference

V2 is the only supported data API. Unversioned V1 routes have been removed
and return `404`; they do not redirect to V2. All tenant/counter data routes
require authentication and the appropriate action scope.

## Contract Guarantees

- Counter increment and set require an `Idempotency-Key` header containing a UUID.
- The key is scoped to the tenant and identifies one logical mutation.
- A retry with the same key and the same canonical request returns the original
  result without applying the mutation again.
- Reusing a key within the same tenant for different counter, operation, or delta data returns
  `409 IDEMPOTENCY_KEY_REUSED`.
- A successful mutation means PostgreSQL committed the counter update and its
  immutable operation record.
- Mutation responses identify whether the result was replayed.

## Authentication

All data routes require an API key. The environment administrator key remains
accepted when `LEGACY_API_KEY_ENABLED=true`; that setting controls the key,
not API-version availability. Tenant-scoped keys use the format
`ck_{credential_id}.{secret}` and are returned only once when created.

Tenant-scoped credentials are authorized by both their tenant and their action
scopes. Supported scopes are `metadata:read`, `metadata:write`, `tenant:read`, `counter:read`, `counter:list`,
`counter:create`, `counter:increment`, `counter:adjust`, and
`counter:history`. `counter:read` permits reading one counter by ID;
`counter:list` separately permits enumerating a tenant's counters and is not
included in the default tenant credential scopes.

Creating a tenant or managing credentials requires an administrator key. A
tenant credential can create counters only when explicitly granted
`counter:create` through `POST /v2/tenants/{tenant_id}/counters`. The default tenant credential scopes do
not include `counter:list`, `counter:create`, or `counter:adjust`.

## Create Tenant

`POST /v2/tenants` requires an administrator key and a JSON body:

```json
{"label":"production"}
```

Returns `201` with `tenant_id`, `label`, `created_at`, and `updated_at`.
Labels are required and globally unique. An optional UUID `Idempotency-Key`
replays the original response for 24 hours.

## Get Tenant

`GET /v2/tenants/{tenant_id}` requires `tenant:read` on that tenant or an
administrator key. Returns `200` with the tenant representation; a missing
tenant returns `404 TENANT_NOT_FOUND`.

## Create Counter

`POST /v2/tenants/{tenant_id}/counters` requires `counter:create` on that tenant
or an administrator key. Example JSON body:

```json
{"label":"daily_steps","initial_value":0,"max_delta":50}
```

`label` is optional and unique within the tenant. `initial_value` defaults to
zero. `max_delta` defaults to 50 and must be positive. Returns `201` with the
counter representation, including its UUID, tenant ID, label, value, maximum
delta, and timestamps. Creation and the initial-value operation are atomic.
An optional UUID `Idempotency-Key` replays the original response for 24 hours.

## List Counters

`GET /v2/tenants/{tenant_id}/counters?limit=20&cursor=...` requires
`counter:list` on that tenant or an administrator key. `counter:read` alone
does not permit listing. `limit` defaults to 20 and must be between 1 and 100.
Returns `200` with `counters` and a nullable `next_cursor`; pass that opaque
cursor unchanged to fetch the next page.

## Get Counter

Returns the current counter representation. Requires a tenant-scoped key with
the `counter:read` scope.

```http
GET /v2/tenants/{tenant_id}/counters/{counter_id}
X-API-Key: tenant-scoped-key
```

Counter reads may
be served from a bounded read-through cache: a cache miss loads from PostgreSQL,
and successful mutations invalidate the cached entry after commit. With the
in-memory backend, a write handled by another API replica may not be visible
until the configured TTL expires. Configure the cache backend, capacity, and
TTL as described in the deployment guide. Cache failures fall back to
PostgreSQL.

### Response

```json
{
  "counter_id": "01912345-6789-7000-8000-000000000002",
  "tenant_id": "01912345-6789-7000-8000-000000000001",
  "label": "daily_steps_2026-09-25",
  "value": 8421,
  "max_delta": 100000,
  "created_at": "2026-09-25T08:00:00Z",
  "updated_at": "2026-09-25T12:01:00Z"
}
```

## Increment Counter

Increments a counter by a positive delta.

### Request

```http
POST /v2/tenants/{tenant_id}/counters/{counter_id}/inc?delta=5
X-API-Key: tenant-scoped-key
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

Increments require a tenant credential with `counter:increment` or an
administrator key. Anonymous increments are not supported.

### Errors

| Status | Code | Meaning |
|--------|------|---------|
| `400` | `IDEMPOTENCY_KEY_REQUIRED` | The request has no `Idempotency-Key` header |
| `400` | `INVALID_IDEMPOTENCY_KEY` | The key is not a valid UUID |
| `400` | `INVALID_DELTA` | Delta is non-positive or malformed |
| `400` | `DELTA_EXCEEDS_MAXIMUM` | Delta exceeds the counter's configured maximum |
| `400` | `COUNTER_OVERFLOW` | The resulting counter value is outside the supported range |
| `404` | `COUNTER_NOT_FOUND` | Counter does not exist for the tenant |
| `409` | `IDEMPOTENCY_KEY_REUSED` | Key was used for different request data |
| `409` | `OPERATION_IN_PROGRESS` | The operation is already being completed |
| `503` | `SERVICE_UNAVAILABLE` | Mutation could not reach a usable database |

## Set Counter Value

The V2 set operation uses the same mandatory idempotency key and durable
operation contract. It records an audited adjustment from the current value to
the requested value; it does not bypass the mutation history. Requires
`counter:adjust` or an administrator key. The `value` field is required.

```http
POST /v2/tenants/{tenant_id}/counters/{counter_id}/set
X-API-Key: your-api-key
Idempotency-Key: 01912345-6789-7000-8000-000000000004
Content-Type: application/json

{
  "value": 100
}
```

### Response

```json
{
  "operation_id": "01912345-6789-7000-8000-000000000004",
  "counter_id": "01912345-6789-7000-8000-000000000002",
  "delta": 53,
  "value": 100,
  "replayed": false,
  "updated_at": "2026-04-07T12:03:00Z"
}
```

An exact retry returns the same result with `replayed: true`.

## Operation History

Returns completed operations for a counter in reverse chronological order.
History requires a tenant-scoped key with `counter:history` or an administrator key.

```http
GET /v2/tenants/{tenant_id}/counters/{counter_id}/operations?limit=50&cursor=...
X-API-Key: your-api-key
```

`limit` defaults to `50` and may range from `1` to `100`. The cursor is opaque;
pass the returned `next_cursor` unchanged to retrieve the next page.

### Response

```json
{
  "operations": [
    {
      "operation_id": "01912345-6789-7000-8000-000000000003",
      "counter_id": "01912345-6789-7000-8000-000000000002",
      "kind": "increment",
      "delta": 5,
      "value_before": 42,
      "value_after": 47,
      "metadata": {},
      "created_at": "2026-04-07T12:02:00Z",
      "completed_at": "2026-04-07T12:02:00Z"
    }
  ],
  "next_cursor": null
}
```

The operation-history response excludes request hashes and other internal
fields. The operator command `--reconcile` verifies that completed operation
deltas equal each stored counter value and that each counter has exactly one
`initial_value` operation.

## Counter metadata

Each counter has at most one independent, optional JSON document. Metadata is
stored separately, owned by the same tenant, and deleted automatically when its
counter is deleted. It is not included in counter GET/list responses, cached
counts, or increment/set operation history. The `metadata` field already present
on operation history describes that operation; it is unrelated to this document.

GET requires `metadata:read`; PUT/DELETE require `metadata:write`.
Administrator keys are also accepted. Neither scope is included in default
tenant credentials, and existing credentials keep their current grants. A
writer receives the document it writes without gaining permission to GET it.

### Read

```http
GET /v2/tenants/{tenant_id}/counters/{counter_id}/metadata
X-API-Key: tenant-key-with-metadata-read
```

```http
HTTP/1.1 200 OK
ETag: "b858fc63-a09a-4ec2-922e-4e91629122dd"
Cache-Control: no-store
Content-Type: application/json
```

```json
{
  "tenant_id": "550e8400-e29b-41d4-a716-446655440000",
  "counter_id": "41537842-e463-45af-9ac9-8e4639aa85d2",
  "data": {"color": "blue", "tags": ["daily"]},
  "version": "b858fc63-a09a-4ec2-922e-4e91629122dd",
  "created_at": "2026-10-19T10:00:00Z",
  "updated_at": "2026-10-19T10:00:00Z"
}
```

No document returns `404 METADATA_NOT_FOUND`; a nonexistent counter returns
`404 COUNTER_NOT_FOUND`. JSON `null` is a valid, present document with
`"data": null`. ETags are opaque, case-sensitive tokens; send the quoted value
exactly as received. GET always returns a full representation and does not
implement conditional `304` responses.

### Create or replace

```http
PUT /v2/tenants/{tenant_id}/counters/{counter_id}/metadata
X-API-Key: tenant-key-with-metadata-write
Content-Type: application/json
If-None-Match: *
Idempotency-Key: 55bed27f-c8ba-4f93-a013-1b97816fbbef

{"color":"blue","tags":["daily"]}
```

Send the raw JSON value, not the response envelope. Objects, arrays, strings,
numbers, booleans, and `null` are accepted. Maximum JSON size is 65,536 bytes
including whitespace, with nesting at most 64 object/array levels. PostgreSQL
JSONB normalizes object ordering/whitespace and duplicate properties; it rejects
values it cannot represent (for example `\u0000` and numbers outside its numeric
range). Large integers are retained without float64 conversion. The server's
configured request body limit may impose a lower cap.

Creation requires `If-None-Match: *` and returns `201` with the envelope and ETag.
Replacement requires `If-Match: "<current-version>"` and returns `200` with a new
ETag, even for identical data. Omitted properties are removed: this is full
replacement. Do not send both conditional headers.

### Delete

```http
DELETE /v2/tenants/{tenant_id}/counters/{counter_id}/metadata
X-API-Key: tenant-key-with-metadata-write
If-Match: "b858fc63-a09a-4ec2-922e-4e91629122dd"
Idempotency-Key: 7eb5ef15-1f20-4da5-a690-352b1b0283e3
```

Returns `204` with no body and leaves the counter value unchanged. A recreated
document gets a fresh version so an old ETag cannot modify it.

For writes, absent conditions return `428 PRECONDITION_REQUIRED`; malformed
conditions (including weak/list/wildcard If-Match) return
`400 INVALID_PRECONDITION`. A stale ETag, create against an existing document,
or replace/delete against an absent document returns `412 PRECONDITION_FAILED`.
Invalid JSON/JSONB or excessive nesting returns `400 INVALID_METADATA`;
oversized JSON returns `413 METADATA_TOO_LARGE` within the API handler.

`Idempotency-Key` is optional for PUT/DELETE. The shared 24-hour replay contract
applies, including reauthorization on every retry. Retrying the same key, body,
and conditional headers returns the original status/body/ETag even after a
later edit; that ETag can therefore be stale. Changed conditional headers with
the same key return `409 IDEMPOTENCY_KEY_REUSED`. Failed preconditions do not
reserve keys. GET current metadata before starting a new edit.
Metadata responses use `Cache-Control: no-store`.

## Tenant Credentials

The administrator may create, rotate, and revoke tenant-scoped credentials.
The raw key is returned only in the create or rotate response; store it
securely because it cannot be recovered later.

All credential routes, including administrator credential creation, accept an
optional UUID `Idempotency-Key`. Retrying with the same authenticated caller,
URL, key, and request within 24 hours prevents duplicate issuance or revocation.
Retries return `Idempotency-Replayed: true`; creation/rotation replays omit
`api_key` and return only the original non-secret fields. If the original secret
was lost, revoke the returned credential ID and create a replacement using a
new key. Omitting the header preserves existing behavior. The required-header
rule for V2 counter increment/set does not apply to credential routes.

See [idempotency coverage](api.md#idempotency-coverage) for key scoping,
conflicts, failure handling, and expiration.

### Create

```http
POST /v2/tenants/{tenant_id}/credentials
X-API-Key: administrator-key
Content-Type: application/json

{
  "scopes": ["counter:read", "counter:list", "counter:create", "counter:increment", "counter:adjust", "counter:history"]
}
```

The `scopes` field is optional and defaults to read, increment, and history.
Include `counter:create` if this key should create counters. An optional `expires_at` timestamp can limit credential
lifetime.

### Rotate

```http
POST /v2/tenants/{tenant_id}/credentials/{credential_id}/rotate
X-API-Key: administrator-key
```

Rotation creates a new key with the same scopes and leaves the old key valid
until it is explicitly revoked, allowing overlap during client migration.

### Revoke

```http
POST /v2/tenants/{tenant_id}/credentials/{credential_id}/revoke
X-API-Key: administrator-key
```

Credential lifecycle events are audited without storing raw secrets.

### Create Administrator Credential

During migration, an existing administrator can provision a managed
administrator credential before disabling the legacy environment key:

```http
POST /v2/admin/credentials
X-API-Key: administrator-key
Content-Type: application/json

{
  "scopes": []
}
```

Administrator credentials are not tenant-scoped and inherit all supported
action scopes. The legacy key should remain enabled until this credential has
been distributed to administrators and verified.

## Migrating from V1

V1 is retired. Replace unversioned URLs with the V2 equivalents; there are no
compatibility aliases or redirects. Counter and tenant IDs remain unchanged.

| Retired route | Supported route | Authorization |
|---|---|---|
| `POST /tenants` | `POST /v2/tenants` | Administrator |
| `GET /tenants/{tenant_id}` | `GET /v2/tenants/{tenant_id}` | `tenant:read` or administrator |
| `POST /tenants/{tenant_id}/counters` | `POST /v2/tenants/{tenant_id}/counters` | `counter:create` or administrator |
| `GET /tenants/{tenant_id}/counters` | `GET /v2/tenants/{tenant_id}/counters` | `counter:list` or administrator |
| `GET /tenants/{tenant_id}/counters/{counter_id}` | `GET /v2/tenants/{tenant_id}/counters/{counter_id}` | `counter:read` or administrator |
| `POST /tenants/{tenant_id}/counters/{counter_id}/inc` | `POST /v2/tenants/{tenant_id}/counters/{counter_id}/inc` | `counter:increment` or administrator; required UUID idempotency key |
| `POST /tenants/{tenant_id}/counters/{counter_id}/set` | `POST /v2/tenants/{tenant_id}/counters/{counter_id}/set` | `counter:adjust` or administrator; required UUID idempotency key |

Tenant credentials must belong to the URL's tenant. Missing/invalid credentials
return `401`; insufficient scopes or another tenant's credential return `403`.
Grant `tenant:read` explicitly to clients that read tenant details; existing
credential scopes are not changed automatically. Creation and credential
lifecycle endpoints retain optional idempotency keys.

Creation replay records are scoped to the caller and URL. Finish outstanding
V1 creation retries before switching paths: the old path's replay record is
not reused at the new V2 URL. Existing increment/set operation history and
idempotency keys remain tenant-scoped and are preserved.

## Reads and Consistency

Counter reads may use the configured bounded cache. A PostgreSQL cache miss
reads from the primary; a cached result may be stale until invalidation or TTL
expiry, particularly across replicas using independent in-memory caches.
Tenant details and counter listing read PostgreSQL directly.
