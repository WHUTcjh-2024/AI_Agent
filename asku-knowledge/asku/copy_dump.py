"""Decode PostgreSQL COPY text as data, without executing its SQL."""

import re
from collections import defaultdict
from pathlib import Path


def read_copy(path: Path) -> dict[str, list[dict]]:
    tables = defaultdict(list)
    table = None
    columns = []
    escapes = {
        "n": "\n",
        "r": "\r",
        "t": "\t",
        "b": "\b",
        "f": "\f",
        "v": "\v",
        "\\": "\\",
    }

    def decode(value):
        if value == r"\N":
            return None
        return re.sub(
            r"\\([0-7]{1,3}|.)",
            lambda m: chr(int(m[1], 8)) if m[1].isdigit() else escapes.get(m[1], m[1]),
            value,
        )

    with path.open(encoding="utf-8", errors="strict") as stream:
        for line in stream:
            line = line.rstrip("\r\n")
            if table:
                if line == r"\.":
                    table = None
                    continue
                values = line.split("\t")
                if len(values) != len(columns):
                    raise ValueError(f"COPY column mismatch in {table}")
                tables[table].append(dict(zip(columns, map(decode, values))))
            else:
                match = re.fullmatch(
                    r"COPY ((?:knowledge|cck)\.\w+) \((.+)\) FROM stdin;", line
                )
                if match:
                    table, columns = match[1], match[2].split(", ")
    if table:
        raise ValueError("Incomplete COPY section")
    return dict(tables)
