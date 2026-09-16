# Counter API V2 Reference

V2 is the stricter API contract for new clients. Existing unversioned routes
remain V1 and continue to support current clients.

> V2 routes are introduced incrementally. An endpoint is available only after
> its implementation has been deployed.

## Contract Guarantees

- Every V2 counter mutation requires an `Idempotency-Key` header containing a UUID.
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
contract explicitly documents a change. The legacy environment administrator
key remains accepted during migration. Tenant-scoped keys use the format
`ck_{credential_id}.{secret}` and are returned only once when created.

Tenant-scoped credentials are authorized by both their tenant and their action
scopes. Supported scopes are `tenant:read`, `counter:read`,
`counter:create`, `counter:increment`, `counter:adjust`, and
`counter:history`.

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

V2 increments require a credential with the `counter:increment` scope. V1
increments remain compatible with the existing public route policy.

### Errors

| Status | Code | Meaning |
|--------|------|---------|
| `400` | `IDEMPOTENCY_KEY_REQUIRED` | The request has no `Idempotency-Key` header |
| `400` | `INVALID_IDEMPOTENCY_KEY` | The key is not a valid UUID |
| `400` | `INVALID_DELTA` | Delta is missing, non-positive, or malformed |
| `400` | `DELTA_EXCEEDS_MAXIMUM` | Delta exceeds the counter's configured maximum |
| `400` | `COUNTER_OVERFLOW` | The resulting counter value is outside the supported range |
| `404` | `COUNTER_NOT_FOUND` | Counter does not exist for the tenant |
| `409` | `IDEMPOTENCY_KEY_REUSED` | Key was used for different request data |
| `409` | `OPERATION_IN_PROGRESS` | The operation is already being completed |
| `503` | `SERVICE_UNAVAILABLE` | Mutation could not reach a usable database |

## Set Counter Value

The V2 set operation uses the same mandatory idempotency key and durable
operation contract. It records an audited adjustment from the current value to
the requested value; it does not bypass the mutation history.

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
History is tenant-scoped and requires the API key.

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

## Tenant Credentials

The administrator may create, rotate, and revoke tenant-scoped credentials.
The raw key is returned only in the create or rotate response; store it
securely because it cannot be recovered later.

### Create

```http
POST /v2/tenants/{tenant_id}/credentials
X-API-Key: administrator-key
Content-Type: application/json

{
  "scopes": ["counter:read", "counter:increment", "counter:history"]
}
```

The `scopes` field is optional and defaults to read, increment, and history.
An optional `expires_at` timestamp can limit credential lifetime.

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

## Reads and Consistency

V2 reads are strong while served by the PostgreSQL primary. Future stale-capable
or striped reads must expose their freshness/version metadata explicitly; they
must not be presented as exact immediate counter values.
