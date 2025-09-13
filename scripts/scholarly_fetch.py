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
      "scholar_cluster_id": "1234567890",
      "num_citations": 45,
      "citedby_url": "https://scholar.google.com/scholar?cites=1234567890"
    },
    ...
  ]
]
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
            filled = scholarly.fill(p)  # fill each pub to get full metadata
            bib = filled.get("bib", {}) or {}

            # authors -> array
            authors_raw = bib.get("author") or ""
            authors = (
                [a.strip() for a in authors_raw.split(" and ") if a.strip()]
                if authors_raw else []
            )

            # year (int)
            year_val = _to_int(bib.get("pub_year"))

            # scholar identifiers
            cluster_id = str(
                filled.get("cites_id") or filled.get("container_id") or ""
            ) or None

            # citation meta
            num_citations = _to_int(
                filled.get("num_citations") or filled.get("num_citations_all")
            )
            citedby_url = filled.get("citedby_url")
            if not citedby_url and cluster_id:
                citedby_url = f"https://scholar.google.com/scholar?cites={cluster_id}"

            results.append({
                "title": bib.get("title") or "",
                "authors": authors,
                "venue": bib.get("venue"),
                "year": year_val,
                "url": filled.get("eprint_url") or filled.get("pub_url"),
                "doi": bib.get("doi"),
                "scholar_cluster_id": cluster_id,
                "num_citations": num_citations,
                "citedby_url": citedby_url,
            })
        except Exception:
            # If one paper fails to fill/parse, skip and continue
            continue

    print(json.dumps(results, ensure_ascii=False))


if __name__ == "__main__":
    try:
        main()
    except Exception:
        # On blocking or transient errors, return empty list
        print("[]")
