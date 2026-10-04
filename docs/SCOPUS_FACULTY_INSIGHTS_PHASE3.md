# Faculty research insights — Phase 3 API review

**READY FOR MANAGER REVIEW.** Phase 3 API implementation is complete on `codex/faculty-research-insights`, based on accepted Phase 2 commit `3345550`. Validation completed **5 October 2026, 04:29 Asia/Bangkok** (4 October 2026, 21:29 UTC). Changes remain uncommitted for manager acceptance/commit. Frontend is clean and unchanged on the same branch. No shared database writes, migration/backfill application, Scopus requests, branch changes, pushes, subagents, manager messages, or automations were performed.

## Result

Two GET endpoints now expose core-only faculty insights and server-paged evidence under the same `ui.page.admin.research_dashboard.view` permission:

- `/api/v1/admin/scopus/dashboard/faculty-insights`
- `/api/v1/admin/scopus/dashboard/faculty-insights/drilldown`

Both use the existing dashboard faculty/eligible-author gate, shared filter implementation, publication-year expression, and selected metric-year rules. The existing mixed dashboard endpoints continue to use their original expressions and behavior. ThaiJO and benchmark data are excluded from the new endpoints.

One unique filtered document population supplies all international counts/percentages, foreign partner memberships, exclusive faculty-role counts/percentages, and every international-status × role cross-tab cell. The same aggregates are returned per observed year, plus an explicit undated bucket. All percentages use documented denominators and JSON null for zero denominators.

Role classification requires a successful current `complete`/`no_correspondence` document status even for First. A trustworthy First beats unknown flags on another eligible author; otherwise unknown flags block Corresponding/Co-author. First + Corresponding + Co links count once as First. Duplicate user matches are eliminated, and conflicting duplicate author flags become unknown conservatively.

Dirty, absent, and obsolete country metadata becomes unknown and contributes no country memberships. Valid incomplete foreign evidence can still establish yes; no requires complete country evidence. The request never selects/parses raw document JSON or repairs data.

Each request begins one read-only repeatable-read transaction and rolls it back after reading. There is no application cache; both return `Cache-Control: no-store`. Deterministic SHA-256 revisions include normalized filters, unique document projections, selected metrics, eligible-role evidence, country metadata/stale markers, and stored country memberships. A supplied stale drilldown revision returns HTTP 409 without documents. Dimensions/page bounds are validated; SQL filter values are bound. Missing migration tables and read failures return safe HTTP 503 rather than misleading zero data. Identical duplicate metric joins deduplicate; conflicting selected metric rows fail unavailable.

Drilldown responses page on the server with stable ascending document ID, full matching total, page/page-size/total-pages, and current role/country evidence. Page size defaults to 50 and is at most 200; the population has no first-200 truncation. The frontend should always send the summary revision and identical dashboard filters, and refresh/restart on 409.

The full frontend contract, field shapes, denominators, query parameters, click-to-drilldown mappings, and error behavior are in [SCOPUS_FACULTY_INSIGHTS_API_CONTRACT.md](SCOPUS_FACULTY_INSIGHTS_API_CONTRACT.md). Frontend wrappers/UI remain deferred to the next assignment.

## Changed files

| File | Change |
|---|---|
| `controllers/admin_scopus_faculty_insights_controller.go` | New uncached snapshot loader, classifiers, aggregations, revision, dimension validation, summary/drilldown handlers |
| `controllers/admin_scopus_dashboard_controller.go` | Extract shared helpers accepting a publication-year expression; original entrypoints retain the exact MariaDB expressions. SQLite equivalents support isolated fixtures |
| `routes/routes.go` | Register both authenticated admin GET routes with the existing research-dashboard permission |
| `controllers/admin_scopus_faculty_insights_test.go` | Pure classifier, denominator/membership, and dimension-validation tests |
| `controllers/admin_scopus_faculty_insights_integration_test.go` | Tagged isolated API fixtures for counts, filters, parity, paging, stale/revision behavior, permission, multi-query snapshots, batching and ambiguity |
| `routes/routes_smoke_test.go` | Require both routes to register without route-tree conflicts |
| `services/scopus_insight_integration_test.go` | Make one existing Phase 2 repair timestamp assertion deterministic by setting a historical baseline instead of relying on different Windows clock ticks; no service behavior changed |
| `docs/SCOPUS_FACULTY_INSIGHTS_API_CONTRACT.md` | Frontend/API handoff contract |
| `docs/SCOPUS_FACULTY_INSIGHTS_PHASE3.md` | This review report |

## Executed checks

| Check | Result |
|---|---|
| `go test ./controllers ./models ./services ./routes ./cmd/scopus-core-insights -count=1`, CGO disabled | **PASS**; models compile, other packages pass |
| `go test -tags insight_integration ./controllers ./services ./routes -count=1`, GOARCH=386/CGO=1/installed `F:\MinGW\bin\gcc.exe` | **PASS**, full isolated suite |
| Existing `go test -tags insight_mariadb ./services -run TestNativeMariaDBCoreInsights -count=1 -v` | **COMPILED / EXPLICIT SKIP**: `SCOPUS_MARIADB_TEST_DSN is unset`; **not a native pass** |
| Backend `git diff --check` | **PASS** |
| Frontend Git status | **Clean**, no UI/wrapper changes |

The primary synthetic API fixture has **246 unique eligible documents**, duplicated user matches and identical metric joins, plus independently excluded test/deleted/non-KKU author documents. Expected and returned international totals are **3 yes / 239 no / 4 unknown**; exclusive role totals are **3 First / 1 Corresponding / 237 Co / 5 unknown**. Partner counts are **Japan 3 / China 1**, with Thailand excluded. These are synthetic fixture results, not live database totals.

Coverage includes:

- Every shared filter individually and combinations, reversed/CE/BE year bounds, title/identifier/journal/keyword/author/affiliation search, citations, OA, aggregation, all supported quality buckets; independent expected fixture counts plus comparison with shared legacy cohort/filter SQL.
- Same-year-complete metric selection, fallback to a previous complete year over a partial current year, cover-display-date year fallback, T1/Q1 exclusivity, missing metrics, conference quality exclusion, and TCI-only zero core results.
- Summary/drilldown total parity; every year/status/role cross-tab cell; partner drilldown parity; undated evidence; empty zero/null output; duplicate joins and First/corresponding/co overlap.
- All **246** documents retrieved across six page requests without duplicates or first-200 truncation; an additional **446-document** fixture crosses the 400-ID relation batch boundary.
- Unknown/stale statuses, nullable flags, obsolete/missing country metadata, valid incomplete foreign evidence, stale country membership suppression, malformed raw payloads that APIs do not parse.
- Deterministic repeated revisions and equivalent list order; revision changes after citations, role flags, dirty metadata, stale memberships, selected metrics and eligible-user changes; stale revision 409 with no documents.
- Invalid dimensions/injection strings/page bounds rejected, out-of-range pages empty, absent authorization/member permission denial and authorized admin reaching the handler, missing schema 503 rather than zero.
- A separate SQLite WAL writer commits citation/role/country-status changes while a read snapshot is open: that snapshot retains all original fields/revision; a later snapshot sees all changes and suppresses stale country rows. This demonstrates multi-query consistency on SQLite, **not MariaDB transaction/locking equivalence**.
- Conflicting selected duplicate metric rows fail safely instead of selecting arbitrary classification/evidence.

The initial full run exposed the previously accepted Phase 2 timestamp test's assumption about two writes receiving distinct clock ticks. The fixture now uses a fixed older timestamp and still checks that dirty same-hash replay replaces it. The final full isolated suite passes. One interrupted command was not executed because automatic approval review hit the account usage limit; the authorized resumed run completed successfully after reset.

## Remaining gates and limits

**Native MariaDB execution remains NOT RUN.** The existing Phase 2 guarded native harness compiles/skips and does not validate the new API endpoints. Actual migration 050 syntax/FKs/triggers/locking plus new endpoint MariaDB SQL, read-only repeatable-read behavior, filter parity, and concurrent revision behavior must be checked on an approved isolated MariaDB 10.11 instance before rollout. No shared migration/backfill has been applied, and no live API country/role parity pass is claimed. The earlier BE 2567–2569 read-only audit baseline remains **226 / 122 yes / 104 no / 0 unknown**, roles **20 / 82 / 124 / 0**, but was not re-measured through these unmigrated live endpoints.

Both APIs materialize the complete filtered lean document projection and batched role/country evidence to compute a consistent revision. Only the requested drilldown page is returned, but DB work and memory are proportional to the full filtered population. This is suitable for the audited 743-document faculty cohort; benchmark larger deployments before changing that strategy. There is no hidden maximum population or silent truncation.

Country/role currentness relies on the accepted Phase 2 dirty triggers and existing role invalidation paths; queries do not reparse or hash raw JSON. Unexpected duplicate metrics with conflicting selected evidence deliberately make the feature unavailable until data is corrected. Native validation and manager acceptance are still required; no deployment readiness claim is made.
