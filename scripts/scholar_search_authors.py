#!/usr/bin/env python3
# -*- coding: utf-8 -*-
# Usage: python3 scripts/scholar_search_authors.py "<QUERY>"

import sys, json
from scholarly import scholarly

def main():
    if len(sys.argv) < 2:
        print("[]"); return
    q = sys.argv[1].strip()
    results = []
    try:
        for a in scholarly.search_author(q):
            a = scholarly.fill(a)  # light fill
            author_id = a.get("scholar_id") or a.get("author_id")
            results.append({
                "author_id": author_id,
                "name": a.get("name"),
                "affiliation": a.get("affiliation"),
                "interests": a.get("interests") or [],
                "citedby": a.get("citedby"),
                "profile_url": f"https://scholar.google.com/citations?user={author_id}" if author_id else None,
            })
            if len(results) >= 10:
                break
    except Exception:
        pass
    print(json.dumps(results, ensure_ascii=False))

if __name__ == "__main__":
    try:
        main()
    except Exception:
        print("[]")
