#!/usr/bin/env python3
"""Restore a PostgreSQL custom-format backup into an isolated database."""

from __future__ import annotations

import argparse
import os
import subprocess
from pathlib import Path


def run(command: list[str]) -> None:
    subprocess.run(command, check=True)


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("backup_file", type=Path)
    args = parser.parse_args()

    restore_db = os.environ.get("RESTORE_TEST_DB")
    restore_url = os.environ.get("RESTORE_DATABASE_URL")
    counter_bin = os.environ.get("COUNTER_BIN", "./counter")
    if not restore_db or not restore_url:
        parser.error("RESTORE_TEST_DB and RESTORE_DATABASE_URL are required")
    if "restore" not in restore_db and "smoke" not in restore_db:
        parser.error("RESTORE_TEST_DB must contain 'restore' or 'smoke'")
    if not args.backup_file.is_file():
        parser.error(f"backup file does not exist: {args.backup_file}")

    created = False
    try:
        run(["createdb", restore_db])
        created = True
        run(["pg_restore", "--exit-on-error", "--no-owner", f"--dbname={restore_url}", str(args.backup_file)])

        environment = os.environ.copy()
        environment["DATABASE_URL"] = restore_url
        environment["API_KEY"] = "restore-smoke-key"
        subprocess.run([counter_bin, "--reconcile"], check=True, env=environment)
    finally:
        if created:
            run(["dropdb", "--if-exists", restore_db])
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
