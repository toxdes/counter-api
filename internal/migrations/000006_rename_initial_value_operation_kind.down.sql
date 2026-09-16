ALTER TABLE counter_operations
    DROP CONSTRAINT IF EXISTS counter_operations_kind_check;

UPDATE counter_operations
SET kind = 'opening_balance'
WHERE kind = 'initial_value';

ALTER TABLE counter_operations
    ADD CONSTRAINT counter_operations_kind_check
    CHECK (kind IN ('opening_balance', 'increment', 'set_adjustment'));
