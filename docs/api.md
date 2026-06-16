# Counter API Documentation

## Quick Start

### Base URL

```
http://localhost:8080
```

### Authentication

Admin endpoints require an API key in the `X-API-Key` header:

```
X-API-Key: your-secret-api-key-here
```

Public endpoints do not require authentication but are rate-limited.

### Making Requests

All requests should use `Content-Type: application/json` for request bodies.

## Endpoints

### Admin Endpoints

Admin endpoints require authentication via `X-API-Key` header.

#### Create Tenant

Creates a new tenant.

**Request**

```http
POST /tenants
X-API-Key: your-api-key
Content-Type: application/json

{
  "label": "blog"
}
```

**Response**

```http
201 Created
Content-Type: application/json

{
  "tenant_id": "01912345-6789-7000-8000-000000000001",
  "label": "blog",
  "created_at": "2026-04-07T12:00:00Z",
  "updated_at": "2026-04-07T12:00:00Z"
}
```

**Error Response**

```http
409 Conflict
Content-Type: application/json

{
  "errors": [
    {
      "code": "TENANT_LABEL_EXISTS",
      "message": "A tenant with this label already exists"
    }
  ]
}
```

#### Create Counter

Creates a new counter under a tenant.

**Request**

```http
POST /tenants/{tenant_id}
X-API-Key: your-api-key
Content-Type: application/json

{
  "label": "post_likes",
  "initial_value": 0,
  "max_delta": 100
}
```

**Parameters**

- `label` (string, optional): Counter label for identification
- `initial_value` (integer, optional): Starting value for the counter, defaults to 0
- `max_delta` (integer, optional): Maximum increment allowed per request, defaults to 50, must be >= 1

**Response**

```http
201 Created
Content-Type: application/json

{
  "counter_id": "01912345-6789-7000-8000-000000000002",
  "tenant_id": "01912345-6789-7000-8000-000000000001",
  "label": "post_likes",
  "value": 0,
  "max_delta": 100,
  "created_at": "2026-04-07T12:00:00Z",
  "updated_at": "2026-04-07T12:00:00Z"
}
```

#### List Counters

Lists all counters for a tenant with cursor-based pagination.

**Request**

```http
GET /tenants/{tenant_id}/counters?limit=20&cursor={cursor}
X-API-Key: your-api-key
```

Query parameters:
- `limit` (optional): Number of counters per page. Defaults to `20`, max `100`.
- `cursor` (optional): Opaque pagination token from a previous response's `next_cursor`. Leave empty for the first page.

**Response**

```http
200 OK
Content-Type: application/json

{
  "counters": [
    {
      "counter_id": "01912345-6789-7000-8000-000000000002",
      "tenant_id": "01912345-6789-7000-8000-000000000001",
      "label": "post_likes",
      "value": 42,
      "max_delta": 100,
      "created_at": "2026-04-07T12:00:00Z",
      "updated_at": "2026-04-07T12:01:00Z"
    }
  ],
  "next_cursor": "eyJjcmVhdGVkX2F0Ijoi..."
}
```

`next_cursor` is `null` when all counters have been returned.

#### Set Counter Value

Sets a counter to a specific value. Requires API key.

**Request**

```http
POST /tenants/{tenant_id}/counters/{counter_id}/set
X-API-Key: your-api-key
Content-Type: application/json

{
  "value": 100
}
```

**Response**

```http
200 OK
Content-Type: application/json

{
  "counter_id": "01912345-6789-7000-8000-000000000002",
  "value": 100,
  "updated_at": "2026-04-07T12:03:00Z"
}
```

### Public Endpoints

Public endpoints do not require authentication.

#### Get Tenant

Retrieves a tenant by ID.

**Request**

```http
GET /tenants/{tenant_id}
```

**Response**

```http
200 OK
Content-Type: application/json

{
  "tenant_id": "01912345-6789-7000-8000-000000000001",
  "label": "blog",
  "created_at": "2026-04-07T12:00:00Z",
  "updated_at": "2026-04-07T12:00:00Z"
}
```

#### Get Counter

Retrieves a counter by ID.

**Request**

```http
GET /tenants/{tenant_id}/{counter_id}
```

**Response**

```http
200 OK
Content-Type: application/json

{
  "counter_id": "01912345-6789-7000-8000-000000000002",
  "tenant_id": "01912345-6789-7000-8000-000000000001",
  "label": "post_likes",
  "value": 42,
  "max_delta": 100,
  "created_at": "2026-04-07T12:00:00Z",
  "updated_at": "2026-04-07T12:01:00Z"
}
```

#### Increment Counter

Increments a counter by a specified delta.

**Request**

```http
POST /tenants/{tenant_id}/{counter_id}/inc?delta=5
```

Query parameters:
- `delta` (optional): Positive integer to increment by. Defaults to `1`. Must not exceed the counter's `max_delta` value.

**Validation**

If `delta` exceeds the counter's `max_delta` value:

```http
400 Bad Request
Content-Type: application/json

{
  "errors": [
    {
      "code": "DELTA_EXCEEDS_MAXIMUM",
      "message": "Delta exceeds maximum allowed value"
    }
  ]
}
```

**Response**

```http
200 OK
Content-Type: application/json

{
  "counter_id": "01912345-6789-7000-8000-000000000002",
  "value": 47,
  "updated_at": "2026-04-07T12:02:00Z"
}
```

## Error Codes

| Code | Description |
|------|-------------|
| `TENANT_NOT_FOUND` | Tenant does not exist |
| `TENANT_LABEL_EXISTS` | Tenant label already taken |
| `COUNTER_NOT_FOUND` | Counter does not exist |
| `COUNTER_LABEL_EXISTS` | Counter label already exists for this tenant |
| `RATE_LIMIT_EXCEEDED` | Rate limit exceeded |
| `UNAUTHORIZED` | Invalid or missing API key |
| `INVALID_JSON` | Malformed JSON in request body |
| `INVALID_PARAMETER` | Invalid query or path parameter |
| `INVALID_DELTA` | Delta must be a positive integer |
| `INVALID_UUID` | Invalid UUID format |
| `INVALID_CURSOR` | Malformed pagination cursor |
| `DELTA_EXCEEDS_MAXIMUM` | Increment delta exceeds the counter's max_delta value |

## Rate Limiting

Public endpoints are rate-limited by IP address.

**Default Limits**
- 10 requests per 60 seconds per IP (POST)
- 30 requests per 60 seconds per IP (GET)

**Rate Limit Headers**

All responses include rate limit information:

```
X-RateLimit-Limit: 10
X-RateLimit-Remaining: 7
```

When rate limited, the response includes:

```
429 Too Many Requests
Retry-After: 30
```

## CORS

The API supports CORS for browser-based requests.

**Allowed Origins**

Configure via `CORS_ALLOWED_ORIGINS` environment variable:
- Exact match: `https://example.com`
- Multiple origins: `https://app.example.com,https://admin.example.com`
- Wildcard all: `*`

**Example Browser Request**

```javascript
fetch('http://localhost:8080/tenants/{tenant_id}/{counter_id}/inc?delta=1', {
  method: 'POST',
  headers: {
    'Content-Type': 'application/json',
  },
})
  .then(response => response.json())
  .then(data => console.log(data));
```

## Examples

### Creating a Blog Post Like Counter

```bash
# 1. Create a tenant
curl -X POST http://localhost:8080/tenants \
  -H "X-API-Key: your-api-key" \
  -H "Content-Type: application/json" \
  -d '{"label": "blog"}'

# 2. Create a counter for post likes
curl -X POST "http://localhost:8080/tenants/{tenant_id}" \
  -H "X-API-Key: your-api-key" \
  -H "Content-Type: application/json" \
  -d '{"label": "post_likes", "initial_value": 0}'

# 3. Increment the counter
curl -X POST "http://localhost:8080/tenants/{tenant_id}/{counter_id}/inc?delta=1"

# 4. Get the counter value
curl -X GET "http://localhost:8080/tenants/{tenant_id}/{counter_id}"
```
