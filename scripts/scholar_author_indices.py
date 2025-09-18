#!/usr/bin/env python3
# -*- coding: utf-8 -*-

"""
Usage:
  python3 scripts/scholar_author_indices.py <SCHOLAR_AUTHOR_ID>

Outputs JSON:
{
  "hindex": 12,
  "hindex5y": 10,
  "i10index": 7,
  "i10index5y": 5,
  "citedby_total": 123,
  "citedby_5y": 95,
  "cites_per_year": {"2018": 8, "2019": 22, "2020": 10, ...}
}
"""

import sys
import json
from scholarly import scholarly

def _to_int(v):
    try:
        return int(v)
    except Exception:
        return None

def main():
    if len(sys.argv) < 2 or not sys.argv[1].strip():
        print("{}")
        return

    author_id = sys.argv[1].strip()

    # fill indices and counts so we get h-index, i10-index, cites per year, etc.
    author = scholarly.search_author_id(author_id)
    author = scholarly.fill(author, sections=["indices", "basics", "counts"])

    out = {
      "hindex": _to_int(author.get("hindex")),
      "hindex5y": _to_int(author.get("hindex5y")),
      "i10index": _to_int(author.get("i10index")),
      "i10index5y": _to_int(author.get("i10index5y")),
      "citedby_total": _to_int(author.get("citedby")),
      "citedby_5y": _to_int(author.get("citedby5y")),
      "cites_per_year": author.get("cites_per_year") or {}
    }

    # Ensure keys are strings and values are ints for JSON consistency
    cpy = {}
    for k, v in (out["cites_per_year"] or {}).items():
        try:
            cpy[str(k)] = int(v)
        except Exception:
            continue
    out["cites_per_year"] = cpy

    print(json.dumps(out, ensure_ascii=False))

if __name__ == "__main__":
    try:
        main()
    except Exception:
        print("{}")
