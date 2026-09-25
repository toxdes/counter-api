#!/usr/bin/env python3
"""Unit tests for the operator-run scalability workload tool."""

from __future__ import annotations

import unittest

import scalability_load


class PercentileTests(unittest.TestCase):
    def test_percentile_reports_nearest_rank_value(self) -> None:
        self.assertEqual(scalability_load.percentile([4, 1, 3, 2], 0.95), 4)

    def test_recorded_command_redacts_inline_api_key(self) -> None:
        command = scalability_load.recorded_command(["tool.py", "--api-key", "secret-value", "--workers", "4"])
        self.assertNotIn("secret-value", command)
        self.assertIn("<redacted>", command)
        equals_command = scalability_load.recorded_command(["tool.py", "--api-key=secret-value"])
        self.assertNotIn("secret-value", equals_command)

    def test_remote_targets_require_explicit_classification(self) -> None:
        self.assertTrue(scalability_load.is_loopback_target("http://localhost:8080"))
        self.assertTrue(scalability_load.is_loopback_target("http://127.0.0.1:8080"))
        self.assertFalse(scalability_load.is_loopback_target("https://counter.example.com"))


class CounterHistoryTests(unittest.TestCase):
    def test_history_matches_submitted_operations_and_current_value(self) -> None:
        operations = [
            {"operation_id": "op-2", "kind": "increment", "delta": 3, "value_before": 5, "value_after": 8},
            {"operation_id": "op-1", "kind": "increment", "delta": 5, "value_before": 0, "value_after": 5},
            {"operation_id": "initial", "kind": "initial_value", "delta": 0, "value_before": 0, "value_after": 0},
        ]

        result = scalability_load.verify_counter_history(
            initial_value=0,
            current_value=8,
            operations=operations,
            expected_increment_ids={"op-1", "op-2"},
        )

        self.assertEqual(result["increment_count"], 2)
        self.assertEqual(result["computed_value"], 8)

    def test_history_detects_missing_successful_operation(self) -> None:
        operations = [
            {"operation_id": "initial", "kind": "initial_value", "delta": 0, "value_before": 0, "value_after": 0},
        ]

        with self.assertRaises(scalability_load.CorrectnessError):
            scalability_load.verify_counter_history(
                initial_value=0,
                current_value=0,
                operations=operations,
                expected_increment_ids={"accepted-op"},
            )


if __name__ == "__main__":
    unittest.main()
