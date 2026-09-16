-- Use counter terminology for the operation that establishes its initial value.
ALTER TABLE counter_operations
    DROP CONSTRAINT IF EXISTS counter_operations_kind_check;

UPDATE counter_operations
SET kind = 'initial_value'
WHERE kind = 'opening_balance';

ALTER TABLE counter_operations
    ADD CONSTRAINT counter_operations_kind_check
    CHECK (kind IN ('initial_value', 'increment', 'set_adjustment'));
