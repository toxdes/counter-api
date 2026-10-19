#!/usr/bin/env python3
"""Verify restore role separation and cleanup without running database commands."""

from __future__ import annotations

import os
import subprocess
import tempfile
import unittest
from pathlib import Path
from unittest.mock import patch

import postgres_restore_smoke


class RestoreTests(unittest.TestCase):
    def setUp(self) -> None:
        directory = tempfile.TemporaryDirectory()
        self.addCleanup(directory.cleanup)
        self.backup = Path(directory.name) / "backup.dump"
        self.backup.touch()
        self.environment = {
            "RESTORE_TEST_DB": "counter_restore_test",
            "RESTORE_DATABASE_URL": "postgres://owner:password@localhost/counter_restore_test",
            "RESTORE_RUNTIME_DATABASE_URL": "postgres://runtime:password@localhost/counter_restore_test",
        }

    def invoke(self) -> int:
        with patch.dict(os.environ, self.environment, clear=True), patch(
            "sys.argv", ["postgres_restore_smoke.py", str(self.backup)]
        ):
            return postgres_restore_smoke.main()

    def test_reconcile_uses_runtime_and_restoration_ignores_source_acls(self) -> None:
        with patch.object(postgres_restore_smoke, "run") as run, patch.object(
            postgres_restore_smoke.subprocess, "run"
        ) as reconcile:
            self.assertEqual(self.invoke(), 0)
        commands = [call.args[0] for call in run.call_args_list]
        self.assertIn("--no-acl", commands[1])
        self.assertIn('TO "runtime"', commands[2][-1])
        self.assertEqual(commands[-1], ["dropdb", "--if-exists", "counter_restore_test"])
        self.assertEqual(
            reconcile.call_args.kwargs["env"]["DATABASE_URL"],
            self.environment["RESTORE_RUNTIME_DATABASE_URL"],
        )

    def test_mismatched_target_is_rejected_before_creation(self) -> None:
        self.environment["RESTORE_RUNTIME_DATABASE_URL"] = "postgres://runtime@localhost/production"
        with patch.object(postgres_restore_smoke, "run") as run, self.assertRaises(SystemExit):
            self.invoke()
        run.assert_not_called()

    def test_same_owner_and_runtime_role_is_rejected(self) -> None:
        self.environment["RESTORE_RUNTIME_DATABASE_URL"] = self.environment["RESTORE_DATABASE_URL"]
        with patch.object(postgres_restore_smoke, "run") as run, self.assertRaises(SystemExit):
            self.invoke()
        run.assert_not_called()

    def test_reconcile_failure_still_drops_only_created_database(self) -> None:
        with patch.object(postgres_restore_smoke, "run") as run, patch.object(
            postgres_restore_smoke.subprocess, "run", side_effect=subprocess.CalledProcessError(1, "counter")
        ), self.assertRaises(subprocess.CalledProcessError):
            self.invoke()
        self.assertEqual(run.call_args.args[0], ["dropdb", "--if-exists", "counter_restore_test"])


if __name__ == "__main__":
    unittest.main()
