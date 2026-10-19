-- Non-counter mutations share a durable replay record with their writes.
-- Response bodies never contain issued API-key secrets.
CREATE TABLE idempotency_requests (
    scope TEXT NOT NULL,
    request_key UUID NOT NULL,
    request_hash BYTEA NOT NULL,
    status_code INTEGER NOT NULL,
    content_type TEXT NOT NULL,
    response_body BYTEA NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at TIMESTAMPTZ NOT NULL DEFAULT NOW() + INTERVAL '24 hours',
    PRIMARY KEY (scope, request_key)
);

CREATE INDEX idx_idempotency_requests_expiry ON idempotency_requests (expires_at);
