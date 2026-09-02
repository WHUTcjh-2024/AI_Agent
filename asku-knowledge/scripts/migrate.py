"""应用数据库迁移。

约束 #1：只允许新增 Migration，禁止 DROP / 重建 / 覆盖现有 knowledge.* 表。
db.assert_migration_is_additive 会在执行前静态拒绝任何破坏性语句。

用法：
    python scripts/migrate.py            # 应用未执行的迁移
    python scripts/migrate.py --dry-run  # 只做安全检查，不落库
"""

from __future__ import annotations

import argparse
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent.parent))

from asku.config import MIGRATIONS_DIR  # noqa: E402
from asku.db import Database, MigrationDriftError, UnsafeMigrationError  # noqa: E402


def main() -> int:
    parser = argparse.ArgumentParser(description="应用 asku-knowledge 数据库迁移")
    parser.add_argument("--dry-run", action="store_true", help="只做安全检查，不落库")
    parser.add_argument("--dsn", default=None, help="PostgreSQL DSN")
    args = parser.parse_args()

    db = Database(dsn=args.dsn).open() if args.dsn else Database().open()
    try:
        applied = db.migrate(MIGRATIONS_DIR, dry_run=args.dry_run)
    except (UnsafeMigrationError, MigrationDriftError) as error:
        print(f"[FAIL] {error}", file=sys.stderr)
        return 2
    finally:
        db.close()

    if applied:
        prefix = "[dry-run] 待应用" if args.dry_run else "已应用"
        for name in applied:
            print(f"{prefix}: {name}")
    else:
        print("所有迁移均已应用，无需变更")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
