# Scopus Ingest Spec (for Codex)

This spec describes how to fetch documents from Scopus (Elsevier API), and how to map each response entry into our MariaDB tables prefixed with `scopus_`.

## Overview

- **Goal**: For a given `scopus_author_id` (read from `users.scopus_id`), fetch all documents using Scopus Search API with `view=COMPLETE`, then upsert into 4 core tables:
  - `scopus_documents`
  - `scopus_authors`
  - `scopus_affiliations`
  - `scopus_document_authors`
- **Config**: Read the API key from table `scopus_config` where `key='api_key'`.

## HTTP Request

- **Endpoint**: `GET https://api.elsevier.com/content/search/scopus`
- **Query params**:
  - `query=AU-ID({scopus_author_id})`
  - `count=25`
  - `start={offset}` (0, 25, 50, …)
  - `view=COMPLETE`
- **Headers**:
  - `Accept: application/json`
  - `X-ELS-APIKey: <value from scopus_config>`

**Example**

```http
GET /content/search/scopus?query=AU-ID(54683571200)&count=25&start=0&view=COMPLETE HTTP/1.1
Host: api.elsevier.com
Accept: application/json
X-ELS-APIKey: <API_KEY>
```

## Example Response (trimmed to 1 entry, `view=COMPLETE`)

```json
{
  "search-results": {
    "opensearch:totalResults": "17",
    "opensearch:startIndex": "0",
    "opensearch:itemsPerPage": "25",
    "entry": [
      {
        "prism:url": "https://api.elsevier.com/content/abstract/scopus_id/105000028608",
        "dc:identifier": "SCOPUS_ID:105000028608",
        "eid": "2-s2.0-105000028608",
        "dc:title": "Toward automated verification of timed business process models...",
        "dc:description": "This paper investigates ... (abstract)",
        "dc:creator": "Dechsupa C.",
        "prism:aggregationType": "Journal",
        "subtype": "ar",
        "subtypeDescription": "Article",

        "prism:publicationName": "Information Sciences",
        "source-id": "15134",
        "prism:issn": "00200255",
        "prism:volume": "710",
        "prism:pageRange": "1-20",
        "prism:coverDate": "2025-08-01",
        "prism:coverDisplayDate": "August 2025",
        "prism:doi": "10.1016/j.ins.2025.122088",
        "pii": "S0020025525002208",

        "citedby-count": "0",
        "openaccess": "0",
        "openaccessFlag": false,

        "authkeywords": "BPMN | timed automata | temporal properties",

        "affiliation": [
          {
            "afid": "60028190",
            "affilname": "Chulalongkorn University",
            "affiliation-city": "Bangkok",
            "affiliation-country": "Thailand",
            "affiliation-url": "https://api.elsevier.com/content/affiliation/affiliation_id/60028190"
          }
        ],

        "author": [
          {
            "authid": "54683571200",
            "authname": "Dechsupa, C.",
            "given-name": "Chatchai",
            "surname": "Dechsupa",
            "initials": "C.",
            "author-url": "https://api.elsevier.com/content/author/author_id/54683571200",
            "afid": ["60028190"]
          },
          {
            "authid": "57212345678",
            "authname": "Kamput, A.",
            "given-name": "Arthit",
            "surname": "Kamput",
            "initials": "A.",
            "author-url": "https://api.elsevier.com/content/author/author_id/57212345678",
            "afid": ["60028190"]
          }
        ],

        "link": [
          { "@ref": "self", "@href": "https://api.elsevier.com/content/abstract/scopus_id/105000028608" },
          { "@ref": "scopus", "@href": "https://www.scopus.com/inward/record.uri?scp=105000028608" }
        ]
      }
    ]
  }
}
```

> Notes
> - In `view=COMPLETE`, `author[]` and `dc:description` (abstract) are present.
> - `authkeywords` is a string separated by `" | "`. Split into an array before storing.

## DB Mapping (MariaDB)

**Upsert strategy (recommended):**
- `scopus_documents`: unique by `eid`
- `scopus_authors`: unique by `scopus_author_id`
- `scopus_affiliations`: unique by `afid`
- `scopus_document_authors`: unique by `(document_id, author_id)`

### 1) scopus_documents

From the response `entry`:

| Column                | Source field                         | Transform |
|-----------------------|--------------------------------------|-----------|
| eid                   | `eid`                                 | as-is     |
| scopus_id             | `dc:identifier`                       | as-is     |
| scopus_link           | `link[]` entry with `@ref = "scopus"` (fallback: `prism:url`) | store first matching `@href` |
| title                 | `dc:title`                            | as-is     |
| abstract              | `dc:description`                      | as-is     |
| aggregation_type      | `prism:aggregationType`               | as-is     |
| subtype               | `subtype`                             | as-is     |
| subtype_description   | `subtypeDescription`                  | as-is     |
| source_id             | `source-id`                           | as-is     |
| publication_name      | `prism:publicationName`               | as-is     |
| issn                  | `prism:issn`                          | as-is     |
| eissn                 | (may not exist)                       | as-is     |
| isbn                  | (may appear in proceedings/books)     | as-is     |
| volume                | `prism:volume`                        | as-is     |
| issue                 | `prism:issueIdentifier`               | as-is     |
| page_range            | `prism:pageRange`                     | as-is     |
| article_number        | `article-number`                      | as-is     |
| cover_date            | `prism:coverDate`                     | parse to `DATE` if possible else `NULL` |
| cover_display_date    | `prism:coverDisplayDate`              | as-is     |
| doi                   | `prism:doi`                           | as-is     |
| pii                   | `pii`                                 | as-is     |
| citedby_count         | `citedby-count`                       | string → int |
| openaccess            | `openaccess`                          | string → tinyint |
| openaccess_flag       | `openaccessFlag`                      | bool → tinyint |
| authkeywords          | `authkeywords`                        | split by `" | "`, store JSON array |
| fund_acr              | (if present)                          | as-is     |
| fund_sponsor          | (if present)                          | as-is     |
| raw_json              | whole `entry`                         | store as JSON (optional) |

### 2) scopus_authors

Loop through `entry.author[]`:

| Column            | Source field   |
|-------------------|----------------|
| scopus_author_id  | `authid`       |
| full_name         | `authname`     |
| given_name        | `given-name`   |
| surname           | `surname`      |
| initials          | `initials`     |
| orcid             | (if present)   |
| author_url        | `author-url`   |

### 3) scopus_affiliations

Loop through `entry.affiliation[]`:

| Column         | Source field            |
|----------------|-------------------------|
| afid           | `afid`                  |
| name           | `affilname`             |
| city           | `affiliation-city`      |
| country        | `affiliation-country`   |
| affiliation_url| `affiliation-url`       |

### 4) scopus_document_authors

For each author (preserve order), link to the document and primary affiliation:

| Column         | How to fill                                                     |
|----------------|------------------------------------------------------------------|
| document_id    | FK to `scopus_documents.id` (found by `eid`)                    |
| author_id      | FK to `scopus_authors.id` (found by `authid`)                   |
| author_seq     | index in `author[]` starting from 1                             |
| affiliation_id | Use first `afid` in `author.afid[]` if present → match `afid`   |

> If an author has multiple affiliations in the same paper but we store only one, choose the first. We can extend later if needed.

## Pagination

1. Fetch the first page with `start=0` and `count=25`.
2. Read `opensearch:totalResults` from the first response.
3. Loop: `start += 25` until `start >= totalResults`.

## Error Handling / Safety

- Treat every field as optional (use safe getters).
- Convert numbers from strings (e.g., `citedby-count`, `openaccess`).
- Date parsing for `cover_date` may fail → default to `NULL`.
- Wrap upsert operations in try/catch per entry so one failure won’t stop the whole batch.

## Optional: Example Upsert Keys

- `scopus_documents.eid` → UNIQUE
- `scopus_authors.scopus_author_id` → UNIQUE
- `scopus_affiliations.afid` → UNIQUE
- `scopus_document_authors (document_id, author_id)` → UNIQUE

## Minimal SQL to read API key

```sql
SELECT value FROM scopus_config WHERE `key`='api_key';
```

---

**End of Spec**