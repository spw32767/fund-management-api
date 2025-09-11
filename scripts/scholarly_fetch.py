#!/usr/bin/env python3
# -*- coding: utf-8 -*-

"""
Usage:
  python3 scripts/scholarly_fetch.py <SCHOLAR_AUTHOR_ID>

Outputs JSON to stdout:
  [
    {
      "title": "...",
      "authors": ["A", "B", ...],
      "venue": "Journal/Conference",
      "year": 2024,
      "url": "https://...",
      "doi": "10.1234/abc...",
      "scholar_cluster_id": "1234567890"
    },
    ...
  ]
"""

import sys
import json
from scholarly import scholarly

def main():
    if len(sys.argv) < 2 or not sys.argv[1].strip():
        print("[]")
        return

    author_id = sys.argv[1].strip()

    # 1) Find author by ID, include publications
    author = scholarly.search_author_id(author_id)
    author = scholarly.fill(author, sections=["publications"])

    results = []
    pubs = author.get("publications", []) or []
    for p in pubs:
        try:
            filled = scholarly.fill(p)
            bib = filled.get("bib", {}) or {}

            # Normalize authors -> array
            authors_raw = bib.get("author") or ""
            authors = [a.strip() for a in authors_raw.split(" and ") if a.strip()] if authors_raw else []

            # Year (int)
            year_val = None
            if bib.get("pub_year"):
                try:
                    year_val = int(bib["pub_year"])
                except Exception:
                    year_val = None

            results.append({
                "title": bib.get("title") or "",
                "authors": authors,
                "venue": bib.get("venue"),
                "year": year_val,
                "url": filled.get("eprint_url") or filled.get("pub_url"),
                "doi": bib.get("doi"),
                "scholar_cluster_id": str(
                    filled.get("cites_id") or filled.get("container_id") or ""
                ),
            })
        except Exception:
            # If one paper fails to fill/parse, skip and continue
            continue

    print(json.dumps(results, ensure_ascii=False))

if __name__ == "__main__":
    try:
        main()
    except Exception:
        # In case of blocking or transient error, return empty array
        print("[]")
