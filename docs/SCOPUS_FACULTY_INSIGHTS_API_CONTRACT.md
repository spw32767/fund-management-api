# Faculty insights API contract — Phase 3

Contract version: `faculty-insights-v1`. Both endpoints are authenticated GET routes under `/api/v1/admin/scopus/dashboard`, with the existing `ui.page.admin.research_dashboard.view` permission and enclosing admin authorization. Both return `Cache-Control: no-store`. Responses use the top-level fields below; there is no `data` wrapper. Frontend implementation/wrappers are deferred to the next assigned phase.

## Shared population and filters

- Source is always `scopus_core`; scope is always `faculty`, including when a caller sends `scope=individual`. Only unique `scopus_documents.id` are counted. No ThaiJO, benchmark, mixed dashboard total, or external Scopus requests.
- Faculty eligibility preserves the dashboard's stored first-affiliation author link: author Scopus ID matches a trimmed user Scopus ID; user is not deleted/test; linked affiliation name is KKU or Faculty of Science KKU. There is no additional role, department, employment-date, or all-affiliation gate. Eligible role authors use exactly this same gate. Duplicate user matches do not multiply eligible links or documents.
- Send the current dashboard query parameters unchanged: `year_start_be`, `year_end_be`, `aggregation_types`, `quality_buckets`, `open_access_mode`, `citation_min`, `citation_max`, `search_title`, `search_doi`, `search_eid`, `search_scopus_id`, `search_journal`, `search_author`, `search_affiliation`, `search_keyword`.
- Parsing remains the shared dashboard parsing: years at least 2400 convert BE to CE, smaller positive years are CE, reversed bounds swap; invalid optional bounds are ignored; lists are comma-separated; OA is `all`/`oa`/`non_oa`; quality accepts T1/Q1/Q2/Q3/Q4/N/A/TCI. TCI-only matches zero core documents. Text uses the existing LIKE semantics (including `%`/`_` wildcards); DOI/EID/Scopus ID use equality. All SQL values are bound.
- Publication year is `COALESCE(YEAR(cover_date), CAST(RIGHT(cover_display_date,4) AS UNSIGNED))`. Null/nonpositive years are displayed as undated. Bounds still use the original expression. Metrics select a complete same-year `doc_type='all'` year, else the latest earlier complete year. Conferences are excluded when any quality filter is selected. T1 means percentile 90–100 and is excluded from Q1–Q4/N/A buckets. Quartile comes from `cite_score_quartile`.

## Summary

`GET /api/v1/admin/scopus/dashboard/faculty-insights`

Example request: `?year_start_be=2567&year_end_be=2569`.

The response contains:

| Field | Meaning |
|---|---|
| `success` | `true` on HTTP 200 |
| `contract_version` | `faculty-insights-v1` |
| `source`, `scope` | `scopus_core`, `faculty` |
| `revision` | Opaque lowercase 64-character SHA-256 revision; send it with drilldowns |
| `totals` | Aggregate for the complete unique filtered population |
| `by_year` | Ascending observed CE years, then an explicit undated bucket, even when empty |

Each `by_year` entry has `bucket` (BE year string, or `undated`), nullable `year_ce` and `year_be`, and the same aggregate fields as `totals`:

```json
{
  "total": 0,
  "international": {"yes": 0, "no": 0, "unknown": 0},
  "international_percent": {"yes": null, "no": null, "unknown": null},
  "roles": {"first": 0, "corresponding": 0, "coauthor": 0, "unknown": 0},
  "role_percent": {"first": null, "corresponding": null, "coauthor": null, "unknown": null},
  "country_role": {
    "yes": {"first": 0, "corresponding": 0, "coauthor": 0, "unknown": 0},
    "no": {"first": 0, "corresponding": 0, "coauthor": 0, "unknown": 0},
    "unknown": {"first": 0, "corresponding": 0, "coauthor": 0, "unknown": 0}
  },
  "partners": []
}
```

All 12 cross-tab cells are present. Counts across international states and exclusive roles each sum to `total`; the cross-tab also sums to `total`. `international_percent` and `role_percent` are percentages from 0–100 using **all filtered documents, including unknown**, as denominator. Empty denominators produce JSON null. No rounding is applied by the API.

Each partner is `{country_key, country_name, documents, percent_international}`. Partners exclude Thailand, count each foreign country at most once per current international document, and sort by documents descending then canonical key ascending. `percent_international` uses `international.yes` as denominator. One document may have several partners, so partner counts/percentages must not be summed into a paper total or displayed as mutually exclusive shares. This denominator policy also applies independently within each year.

## Classification and evidence

Country metadata is current only when its normalizer version equals the running `core-countries-v2` and status is `complete` or `incomplete`. Current true international evidence gives `yes`, even when incomplete. Current false evidence gives `no` only when countries are complete. Everything else is `unknown`. Dirty payload/catalogue, absent metadata, and obsolete versions expose no country memberships. Requests do not parse raw payloads, repair metadata, or write data.

Roles are exclusive: **First > Corresponding > Co-author**, with an explicit unknown state. Only `complete` or `no_correspondence` document role status can establish a role. A reliable positive First flag on an eligible author wins even if another eligible author's flags are unknown. Otherwise any unknown First/Corresponding flag blocks lower roles. Conflicting duplicate links for the same author become unknown for the conflicting flag. A positive Corresponding flag contradicting `no_correspondence` blocks lower roles. Author sequence alone never establishes First.

## Drilldown

`GET /api/v1/admin/scopus/dashboard/faculty-insights/drilldown`

Send **the same dashboard filters as the summary**, plus the dimensions below. Dimensions combine with AND and do not replace the summary filters.

| Parameter | Allowed value/default |
|---|---|
| `revision` | Summary's lowercase SHA-256 hash; technically optional, but frontend should always send it |
| `year_be` | Exact BE bucket from `by_year.bucket`, or `undated`; numeric range 544–10542, converted by subtracting 543 |
| `international_status` | `yes`, `no`, `unknown`; omit for all |
| `faculty_role` | `first`, `corresponding`, `coauthor`, `unknown`; omit for all |
| `country_key` | Canonical lowercase key from a partner/country response, at most 96 ASCII letters/spaces/apostrophes/hyphens, starting with a letter; omit for all |
| `page` | Integer 1–1,000,000; default 1 |
| `page_size` | Integer 1–200; default 50 |

Valid but absent country keys return zero documents. Country selection uses only current normalized memberships; Thailand is supported for domestic/current-evidence exploration. For a partner click, send `country_key` **and** `international_status=yes` so its drilldown total exactly equals the partner count. Numeric `year_be` is a literal BE year here, unlike the shared bounds parser. Use the returned bucket value.

HTTP 200 shape:

```json
{
  "success": true,
  "contract_version": "faculty-insights-v1",
  "source": "scopus_core",
  "scope": "faculty",
  "revision": "<64 lowercase hex characters>",
  "total": 246,
  "page": 1,
  "page_size": 50,
  "total_pages": 5,
  "sort": "document_id_asc",
  "documents": []
}
```

`total` is the full dimension-matching count, independent of page size. Pages use ascending unique document ID. The server returns only the requested page; there is no first-200 population truncation. Pages beyond the last page are empty. Empty results use `documents: []`, `total: 0`, `total_pages: 0`.

Each document includes `document_id`, `eid`, nullable `scopus_id`, `title`, `doi`, `scopus_link`, `publication_name`, `aggregation_type`, nullable `year_ce`/`year_be`, `citations`, `openaccess_flag`, `openaccess`, nullable metric year/quartile/percentile/status, author-role status/check time, `updated_at`, `international_status`, `faculty_role`, `country_evidence_current`, `countries`, `eligible_authors`, and nullable `country_metadata`.

- A country is `{country_key, country_name, provenance}`.
- An eligible-author evidence link is `{link_id, author_id, scopus_author_id, full_name, author_seq, affiliation_id, is_first_author, is_corresponding_author}`. Flags are nullable. Several links may represent one author; classification consolidates them conservatively. User duplicates are removed.
- `country_metadata` contains the Phase 2 status/version/check time, nullable international flag, completeness flags, hashes, provenance, and stored diagnostic/reason JSON **strings**. When `country_evidence_current=false`, those diagnostics describe untrusted/previous evidence; `countries` is empty and the classification is unknown. No raw document payload is included.

## Revision and errors

Each request reads in one `READ ONLY`, `REPEATABLE READ` transaction and rolls back after reading. Revision hashes the normalized shared filters, contract version, ordered unique document projections, selected metrics, eligible-author flags/evidence, metadata status/version/hashes/check time, and ordered stored country rows, including stale rows. Dimension/paging parameters do not alter the revision. Equivalent list order/duplicates and scope normalize; raw search case is retained conservatively. Relevant row changes can invalidate a revision even when aggregate counts remain equal.

| HTTP/code | Required frontend handling |
|---|---|
| 400 `invalid_insight_dimension` | Correct the invalid enum/year/key/hash/page; do not silently retry with a different selection |
| 409 `insight_revision_mismatch` | Discard old pages, refresh summary with the same dashboard filters, and reopen/restart drilldown using its revision |
| 503 `faculty_insights_unavailable` | Show an unavailable state; do not render zero totals. Verify migration 050/database reads. Missing tables, read failures, or conflicting duplicate selected metrics fail safely |
| 401/403 from existing authorization | Follow existing login/permission flow |

409 includes the current `revision` and no documents. All feature errors include `success:false`, `code`, and a sanitized `error`. No endpoint automatically migrates or backfills a database. A missing document-level metadata row after migration is a valid unknown classification, while missing required migration tables is unavailable.

## Implementation limit and rollout

For consistent hashing and classifications, both endpoints load the complete filtered projection/evidence, batch relation reads in groups of 400 document IDs, then aggregate/filter/page on the server. Responses are server-paged, but database work and memory remain proportional to the filtered cohort. This is acceptable for the audited 743-document faculty cohort; benchmark on larger deployments before changing that strategy. No silently truncated maximum population is imposed.

The configured shared development database has not received migration/backfill in this task, so live API country parity is **not claimed**. Native MariaDB 10.11 migration/trigger/FK/locking/read-only snapshot validation remains a rollout gate. The earlier audit baseline for BE 2567–2569 is 226 unique core faculty documents, 122 yes / 104 no / 0 unknown, and 20 First / 82 Corresponding / 124 Co / 0 unknown; it is prior read-only audit evidence, not a newly executed API result.
