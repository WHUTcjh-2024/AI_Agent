import hashlib
import tempfile
import unittest
from pathlib import Path
from unittest.mock import patch

from asku.db import Database, MigrationDriftError, UnsafeMigrationError, assert_migration_is_additive


class MigrationSafetyTests(unittest.TestCase):
    def test_all_repository_migrations_are_additive(self) -> None:
        migration_dir = Path(__file__).resolve().parents[1] / "database" / "migrations"
        for path in migration_dir.glob("*.sql"):
            assert_migration_is_additive(path.read_text(encoding="utf-8"), path.name)

    def test_non_additive_alter_is_rejected(self) -> None:
        with self.assertRaises(UnsafeMigrationError):
            assert_migration_is_additive(
                "ALTER TABLE knowledge.documents ALTER COLUMN title TYPE varchar(20);",
                "unsafe.sql",
            )

    def test_changed_applied_migration_is_rejected(self) -> None:
        script = "CREATE TABLE IF NOT EXISTS crawler.example (id text PRIMARY KEY);"
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "001_example.sql"
            path.write_text(script, encoding="utf-8")
            database = Database()
            with patch.object(database, "applied_migrations", return_value={path.name: "wrong-checksum"}):
                with self.assertRaises(MigrationDriftError):
                    database.migrate(Path(directory), dry_run=True)
            database.close()

        self.assertNotEqual(hashlib.sha256(script.encode()).hexdigest(), "wrong-checksum")


if __name__ == "__main__":
    unittest.main()
