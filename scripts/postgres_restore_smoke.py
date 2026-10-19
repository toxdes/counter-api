#!/usr/bin/env python3
"""Restore a PostgreSQL custom-format backup into an isolated database."""

from __future__ import annotations

import argparse
import os
import subprocess
from pathlib import Path
from urllib.parse import unquote, urlparse


def run(command: list[str]) -> None:
    subprocess.run(command, check=True)


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("backup_file", type=Path)
    args = parser.parse_args()

    restore_db = os.environ.get("RESTORE_TEST_DB")
    restore_url = os.environ.get("RESTORE_DATABASE_URL")
    runtime_url = os.environ.get("RESTORE_RUNTIME_DATABASE_URL")
    counter_bin = os.environ.get("COUNTER_BIN", "./counter")
    if not restore_db or not restore_url or not runtime_url:
        parser.error("RESTORE_TEST_DB, RESTORE_DATABASE_URL, and RESTORE_RUNTIME_DATABASE_URL are required")
    if "restore" not in restore_db and "smoke" not in restore_db:
        parser.error("RESTORE_TEST_DB must contain 'restore' or 'smoke'")
    if not args.backup_file.is_file():
        parser.error(f"backup file does not exist: {args.backup_file}")
    owner = urlparse(restore_url)
    runtime = urlparse(runtime_url)
    if (
        (owner.hostname, owner.port or 5432, owner.path)
        != (runtime.hostname, runtime.port or 5432, runtime.path)
        or unquote(owner.path.lstrip("/")) != restore_db
    ):
        parser.error("both restore URLs must target RESTORE_TEST_DB on the same server")
    if not runtime.username or runtime.username == owner.username:
        parser.error("runtime URL must use a separate, existing restricted role")
    runtime_role = '"' + unquote(runtime.username).replace('"', '""') + '"'

    created = False
    try:
        run(["createdb", restore_db])
        created = True
        run(["pg_restore", "--exit-on-error", "--no-owner", "--no-acl", f"--dbname={restore_url}", str(args.backup_file)])
        # This application's canonical tables live in public. Restore ignores
        # source ACLs and grants only the runtime access needed by reconciliation.
        grants = (
            f"GRANT USAGE ON SCHEMA public TO {runtime_role}; "
            f"GRANT SELECT ON ALL TABLES IN SCHEMA public TO {runtime_role}; "
            f"GRANT EXECUTE ON ALL FUNCTIONS IN SCHEMA public TO {runtime_role};"
        )
        run(["psql", "--set", "ON_ERROR_STOP=1", f"--dbname={restore_url}", "--command", grants])

        environment = os.environ.copy()
        environment["DATABASE_URL"] = runtime_url
        environment["API_KEY"] = "restore-smoke-key"
        subprocess.run([counter_bin, "--reconcile"], check=True, env=environment)
    finally:
        if created:
            run(["dropdb", "--if-exists", restore_db])
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
