# Faculty insights UI refinement — 2026-10-05

The faculty paper dialog now follows the existing Scopus Benchmark dialog layout and searches the entire clicked group before pagination. Role table/chart alignment and structured explanations were refined. The dashboard cohort and classification rules remain those accepted in Phases 3–5.

## Backend contract

`GET /api/v1/admin/scopus/dashboard/faculty-insights/drilldown` accepts optional `drilldown_search` independently of shared dashboard filters. It is trimmed, limited to 200 Unicode code points, rejects invalid UTF-8/control characters, and performs case-insensitive literal substring matching across title, DOI, EID, Scopus ID, publication name, and names of eligible faculty authors. `%` and `_` have no wildcard meaning. Missing values are safe. It does not search raw JSON, abstracts, keywords, or all authors.

Order: load the full filtered read-only repeatable snapshot → validate its revision → match clicked year/international/role/country dimensions → record `scope_total` → search all selected documents → paginate in ascending document ID order. `total` is the matched count, `scope_total` is the pre-search clicked-group count, and `search` echoes the normalized text. The existing summary revision excludes this local search term. Existing page-size bounds (1–200) remain.

The EID projection lacked an explicit GORM `column:eid` mapping, discovered by the new API test. EID/DOI now have explicit column tags so identifiers are populated and searchable. Populating previously empty projected fields changes the computed revision once when the updated backend starts; normal 409 recovery handles an older client summary.

## Frontend behavior

- Reused Benchmark `report/Hint.js` unchanged with section titles and bullet content from `scopus_faculty_insight_hints.mjs`.
- Dialog: white max-width-6xl panel, slate search toolbar, blue sticky table header, expandable metadata/eligible-author rows, matched/group totals, page selector, previous/next and page-size controls. Retains Headless UI focus handling and safe external links.
- Local search retains clicked dimensions, applied dashboard filters and revision; search/clear starts page 1, paging retains text. Aborted/obsolete replies cannot overwrite a later search/clear/close. A 409 refreshes the shared summary once and reopens the same group/search from page 1. Retry preserves the term.
- Role year selector is above both table and chart, explicitly affects only that chart. Desktop top-edge alignment is asserted; mobile stacks them. Expanded mobile metadata remains visible after horizontal table scrolling.
- Tooltips cover scope/cohort, Scopus-only source, applied filters/years, unique counts, three country states, incomplete/dual affiliations, all-paper denominators, overlapping partner countries, exclusive role precedence, current successful XML checks and `no_correspondence`, each donut's denominator, local selectors and cross-tabs. Main card text stays concise.

## Verification

- Normal backend regression: `CGO_ENABLED=0 go test ./controllers ./services ./routes` — PASS.
- Full isolated SQLite regression: `GOARCH=386 CGO_ENABLED=1 CC=F:\MinGW\bin\gcc.exe go test -tags insight_integration ./controllers ./services ./routes` — PASS. Uses temporary synthetic databases, not the configured TEST database.
- New pure test validates supported fields, nil values, Unicode bounds/control/UTF-8 rejection and literal `%_`.
- New API test searches document 246 beyond page 200 by title/Thai text/EID/Scopus ID/DOI, preserves full revision and scope, combines undated/country/international/role dimensions, verifies eligible-author and journal search, empty/cleared/paged responses, 409 rejection before search, and retention of global title filter.
- Frontend `node --test` — 87 PASS, including full 512-paper search, clear/page reset, selection dimensions, unchanged summary, search races/close and 409 term preservation.
- Browser fixture evidence and final build results: see companion frontend `docs/SCOPUS_FACULTY_INSIGHTS_UI_REFINEMENT.md` and `docs/faculty-insights-ui-refinement-evidence/`.

No schema changes, shared database writes, harvesting, backend serving-runtime restart, commit, push or deployment were performed for this refinement. Actual TEST database rollout remains the separate accepted receipt in commit `78a5ae0`. Current visual checks use synthetic fixtures; they do not claim a newly activated live dashboard.

## Changed backend files

1. `controllers/admin_scopus_faculty_insights_controller.go`: local search dimension/response and identifier mappings.
2. `controllers/admin_scopus_faculty_insights_test.go`: search field/bound tests.
3. `controllers/admin_scopus_faculty_insights_integration_test.go`: full-group API fixture tests.
4. This report.
