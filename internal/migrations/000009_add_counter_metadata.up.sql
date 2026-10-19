-- Counter-owned documents: absence of a row means no metadata. A JSON null
-- value is a real document and is distinct from absence. Revisions are UUIDs
-- so deleting/recreating a document cannot reuse an old conditional ETag.
CREATE TABLE counter_metadata (
    tenant_id UUID NOT NULL,
    counter_id UUID NOT NULL,
    data JSONB NOT NULL,
    version UUID NOT NULL DEFAULT gen_random_uuid(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (tenant_id, counter_id),
    FOREIGN KEY (tenant_id, counter_id) REFERENCES counters (tenant_id, id) ON DELETE CASCADE
);
