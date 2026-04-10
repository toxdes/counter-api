# Max Delta Support for Counters

**Date:** 2026-04-10
**Status:** Approved

## Overview

Add a `max_delta` field to counters that limits the maximum increment value allowed per request. This prevents accidental large increments and provides better control over counter updates.

## Requirements

1. Counters must have a `max_delta` value that limits the increment delta
2. `max_delta` is set during counter creation and cannot be changed afterward (immutable)
3. Default `max_delta` value is 50
4. Minimum allowed `max_delta` is 1
5. The `max_delta` value must be included in API responses
6. Increment requests with `delta > max_delta` must return a 400 Bad Request error

## Database Changes

### Migration
Add a `max_delta` column to the `counters` table:

```sql
ALTER TABLE counters ADD COLUMN max_delta INTEGER NOT NULL DEFAULT 50;
```

### Schema
- Column: `max_delta`
- Type: `INTEGER`
- Constraint: `NOT NULL`
- Default: `50`

## API Changes

### Create Counter
**Endpoint:** `POST /tenants/{tenant_id}/counters`

**Request Body:**
```json
{
  "label": "my-counter",
  "initial_value": 0,
  "max_delta": 100
}
```

**Field Details:**
- `label` (string, optional): Counter label
- `initial_value` (integer, optional): Initial counter value, defaults to 0
- `max_delta` (integer, optional): Maximum allowed delta per increment, defaults to 50

**Validation:**
- `max_delta` must be >= 1
- If not provided, defaults to 50

**Response:** 201 Created
```json
{
  "counter_id": "uuid",
  "tenant_id": "uuid",
  "label": "my-counter",
  "value": 0,
  "max_delta": 100,
  "created_at": "2026-04-10T12:00:00Z",
  "updated_at": "2026-04-10T12:00:00Z"
}
```

### Increment Counter
**Endpoint:** `POST /tenants/{tenant_id}/counters/{counter_id}/inc?delta=10`

**Validation Flow:**
1. Parse delta from query parameter (defaults to 1)
2. Validate delta is a positive integer
3. **NEW:** Fetch counter's `max_delta` value
4. **NEW:** Validate `delta <= max_delta`
5. If validation fails, return 400 Bad Request

**Error Response (delta exceeds max):**
```json
{
  "error_code": "DELTA_EXCEEDS_MAXIMUM",
  "message": "Delta exceeds maximum allowed value"
}
```

**Success Response:** 200 OK
```json
{
  "counter_id": "uuid",
  "value": 10,
  "updated_at": "2026-04-10T12:00:00Z"
}
```

### Get Counter
**Endpoint:** `GET /tenants/{tenant_id}/counters/{counter_id}`

**Response:** 200 OK
```json
{
  "counter_id": "uuid",
  "tenant_id": "uuid",
  "label": "my-counter",
  "value": 10,
  "max_delta": 100,
  "created_at": "2026-04-10T12:00:00Z",
  "updated_at": "2026-04-10T12:00:00Z"
}
```

## Model Changes

### Counter Model
```go
type Counter struct {
    ID        string    `json:"counter_id" db:"id"`
    TenantID  string    `json:"tenant_id" db:"tenant_id"`
    Label     string    `json:"label" db:"label"`
    Value     int64     `json:"value" db:"value"`
    MaxDelta  int64     `json:"max_delta" db:"max_delta"`  // NEW
    CreatedAt time.Time `json:"created_at" db:"created_at"`
    UpdatedAt time.Time `json:"updated_at" db:"updated_at"`
}
```

### CreateCounterRequest
```go
type CreateCounterRequest struct {
    Label        string `json:"label"`
    InitialValue int64  `json:"initial_value"`
    MaxDelta     int64  `json:"max_delta"`  // NEW
}
```

**Validation:**
- If `MaxDelta` is provided (non-zero), validate it's >= 1
- If `MaxDelta` is 0 or not provided, default to 50

## Testing Considerations

1. Test counter creation with various `max_delta` values (1, 50, 1000)
2. Test counter creation without `max_delta` (should default to 50)
3. Test counter creation with `max_delta < 1` (should fail validation)
4. Test increment with delta <= max_delta (should succeed)
5. Test increment with delta > max_delta (should return 400)
6. Test that max_delta is immutable (no update endpoint)
7. Test that max_delta is included in all counter responses
