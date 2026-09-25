-- Make counter ownership structural for the operation history foreign key.
ALTER TABLE counters
    ADD CONSTRAINT counters_tenant_id_id_unique UNIQUE (tenant_id, id);

CREATE TABLE counter_operations (
    tenant_id UUID NOT NULL,
    counter_id UUID NOT NULL,
    operation_id UUID NOT NULL,
    kind TEXT NOT NULL CHECK (kind IN ('opening_balance', 'increment', 'set_adjustment')),
    delta BIGINT NOT NULL,
    request_hash BYTEA NOT NULL,
    value_before BIGINT,
    value_after BIGINT,
    actor_id TEXT,
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    state TEXT NOT NULL CHECK (state IN ('pending', 'completed')),
    created_at TIMESTAMPTZ NOT NULL,
    completed_at TIMESTAMPTZ,
    PRIMARY KEY (tenant_id, operation_id),
    FOREIGN KEY (tenant_id, counter_id)
        REFERENCES counters (tenant_id, id)
        ON DELETE RESTRICT
);

CREATE INDEX idx_counter_operations_history
    ON counter_operations (tenant_id, counter_id, created_at DESC, operation_id DESC);

-- Give existing counters a deterministic, repeatable initial-value operation.
INSERT INTO counter_operations (
    tenant_id, counter_id, operation_id, kind, delta, request_hash,
    value_before, value_after, metadata, state, created_at, completed_at
)
SELECT
    c.tenant_id,
    c.id,
    md5('counter-opening:' || c.id::text)::uuid,
    'opening_balance',
    c.value,
    decode(md5('counter-opening:' || c.id::text), 'hex'),
    0,
    c.value,
    '{}'::jsonb,
    'completed',
    c.created_at,
    c.created_at
FROM counters AS c
ON CONFLICT (tenant_id, operation_id) DO NOTHING;
