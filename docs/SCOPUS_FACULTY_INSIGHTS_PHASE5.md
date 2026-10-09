# Faculty research insights — Phase 5 integrated verification

**Subsequent authorized TEST rollout:** [TEST database rollout report](SCOPUS_FACULTY_INSIGHTS_TEST_ROLLOUT.md) records the later actual migration/backfill and saved-data verification. Unapplied-shared-TEST statements below describe the historical Phase 5 state; production remains untouched.

**READY FOR MANAGER REVIEW.** The previously pending native MariaDB gate passed on an isolated, checksum-verified **MariaDB 10.11.16 Windows** process. Native foundation/API checks, authenticated frontend integration, regressions and production preview guards passed. This supersedes the historical Phase 2/3 statements that native execution was pending.

Backend baseline `9f2e20f`; frontend baseline `10f7644`, both on `codex/faculty-research-insights`. Changes remain uncommitted. No shared migration/backfill, shared database writes, Scopus harvesting, deployment, commit/push, branch switch, subagent, manager message or automation was performed.

## Native runtime and cleanup

Used the official [10.11.16 Windows listing](https://dlm.mariadb.com/browse/mariadb_server/10.11.16/winx64-packages/), [ZIP archive](https://dlm.mariadb.com/4559168/MariaDB/mariadb-10.11.16/winx64-packages/mariadb-10.11.16-winx64.zip) and [SHA-256 manifest](https://dlm.mariadb.com/4559170/MariaDB/mariadb-10.11.16/winx64-packages/sha256sums.txt). The downloaded hash matched:

`b1659bd9fe816624632c6b445ce4ad1ed6758a199e35f89e2448aef99828527b`

Followed the official [Windows ZIP instructions](https://mariadb.com/docs/server/server-management/install-and-upgrade-mariadb/installing-mariadb/binary-packages/installing-mariadb-windows-zip-packages) and [mariadb-install-db.exe documentation](https://mariadb.com/docs/server/server-management/install-and-upgrade-mariadb/installing-mariadb/installing-system-tables-mariadb-install-db/mariadb-install-db-exe). Runtime directory: `G:\works-fund-project\.phase5-mariadb`, outside both repos. Initialization omitted the service argument. `mariadbd.exe` ran with `Start-Process -WindowStyle Hidden`, explicit defaults file, **127.0.0.1:33117**, 128 MiB InnoDB buffer pool, maximum 32 connections. No Windows service, global PATH change or system installation.

Harnesses require loopback, actual MariaDB 10.11, matching `scopus_insights_test_*` identity and initially empty schemas; they never read application `.env`/`DB_*`. Used only `scopus_insights_test_phase2` and `scopus_insights_test_api`. Test cleanup removed every owned table; final information-schema check returned **zero tables in both schemas**. After checking the owned PID/executable, graceful shutdown used the absolute portable `mariadb-admin.exe` against port 33117. Browser/frontend dev/production processes were also stopped; synthetic token handshake files were removed. Stopped archive/runtime and empty schemas remain outside the repos for review/reuse.

[Sanitized runtime evidence](audits/faculty_insights_phase5_runtime.json) records provenance/checksum, version, binding and stopped state, without application credentials.

## Executed gates

| Check | Result |
|---|---|
| Prepared `TestNativeMariaDBCoreInsights` | **PASS**, actual MariaDB, 1.05 s |
| New `TestNativeMariaDBFacultyInsights`, including real UI handshake | **PASS**, all subtests, 42.10 s |
| Backend controllers/models/services/routes/CLI regression | **PASS**, models compile; other packages pass |
| Full isolated SQLite controllers/services/routes integration | **PASS**, existing 32-bit GCC |
| Frontend `node --test` | **PASS**, 84/84 |
| Authenticated native API browser integration | **PASS**, 8 checks, 15 API responses, zero page errors/external requests |
| Frontend production build | **PASS**, compile/type validity/static generation; nonfatal cache EPERM warnings |
| Both faculty-insight developer preview paths in built production | **PASS**, HTTP 404 |
| Built `/research-fund-system/admin/research-dashboard` | **PASS**, HTTP 200 route shell; not authenticated live data validation |
| Both repository `git diff --check` | **PASS** |

Standalone ESLint remains unconfigured from Phase 4; no tooling dependencies changed. A sandboxed regression attempt could not read a Go-cache entry; the approved rerun passed. Initial native attempts found harness setup mistakes (multiple statements with a single-statement driver, local-time/UTC session seed) and a browser assertion that incorrectly expected all unknown documents to lack countries. Those fixtures were corrected; no foundation/API behavior defect was found. Saved evidence is from the final passing run.

## What native execution establishes

The prepared foundation harness executed migration 050/reruns, unsigned FKs/cascades, JSON columns, country/AFID insert/update/delete/rename dirty triggers, case/padding/accent separation, partial/full replacement and replay, payload dirty/restore, rollback, concurrent missing-AFID insertion/existing-country updates, forced deadlock rollback, and actual **LOCK IN SHARE MODE** statements. No foundation production fixes were needed.

The API harness executes production MariaDB publication-year/metric-year SQL, actual migration tables/triggers, handlers and auth/permission middleware. Its baseline has **246 unique eligible documents**, duplicate user matches/identical metric joins and three independently excluded documents. Independent expected and returned totals match:

- International: **3 yes / 239 no / 4 unknown**.
- Role: **3 First / 1 Corresponding / 237 Co-author / 5 unknown**.
- Partners: **Japan 3 / China 1**, excluding Thailand.
- Four year buckets include missing publication year. Every year × international × role drilldown matches the summary cell. Pages of **200 + 46** return 246 distinct IDs.

31 native filter cases cover CE/BE/reversed year ranges, aggregation, OA/non-OA, citations, title/DOI/EID/Scopus ID/journal/keyword/author/affiliation searches, every quality bucket, combined filters, no-result/injection-like text. All summary/drilldown totals agree. Existing isolated suites cover further invalid dimensions/page bounds, conflicting metrics, revision dependencies, batching and legacy cohort parity.

An open **read-only REPEATABLE READ** snapshot retains the original revision/citation/role/countries after a second connection commits citation/role/raw-payload changes. A write attempted inside it returns native **1792**. A later snapshot sees all new state, suppresses stale memberships and changes revision; old-revision drilldown returns **409 without documents**. A catalogue-country update fires actual dirty triggers and removes stale Japan memberships from results. Removing the owned insight table makes the real summary return **503 without totals**.

HTTP authentication returns 401 for absent/invalid JWTs and missing/expired sessions, 200 for an active database-backed session, and 403 for a database member whose signed token claims admin. Existing legacy sessionless JWT behavior also returns 200. Browser integration uses the active synthetic session and actual `AuthMiddleware`/`RequirePermission`. The httptest server mounts the real handlers at actual paths; it does not boot the whole application or exercise production login/refresh infrastructure. The passing route smoke suite checks full application route registration.

## Frontend integration and semantics

The new development page supplies **no injected API or fixture fallback**. `NEXT_PUBLIC_API_URL` was overridden only for its local dev process. Browser traffic is limited to frontend/API loopback origins. The synthetic JWT is seeded only into the disposable browser; middleware validates its database-backed session, user/email and permission.

Eight browser checks cover real summary totals, pagination beyond 200, Japan parity, unknown evidence/safe links, SQL mutation causing **409 → summary refresh → page 1**, draft versus applied BE filters, actual empty-year/zero denominators and mobile width. Every drilldown sends a 64-character summary revision. Desktop/mobile screenshots were visually inspected. [Native API JSON and EXPLAIN](audits/faculty_insights_phase5_native_api.json) includes sanitized browser results; frontend evidence retains four screenshots and production guards.

One unknown document has valid **incomplete Thailand evidence**. It displays that current evidence while remaining unknown; absent/dirty/obsolete evidence displays no current countries. Incomplete domestic evidence does not become a domestic assertion.

The Thai role explanation explicitly states that **Corresponding counts papers without a faculty First**, and First → Corresponding → Co-author is an exclusive deduplication rule, **not an importance ranking**. Current successful checks are required; a confirmed First wins, otherwise unresolved flags block lower roles. Unknown states/missing years/zero denominators remain explicit.

Source review confirms the actual faculty dashboard places the cards immediately after Faculty Quartile History and before H-index, derives `facultyInsightQuery` from `appliedFilters` and passes existing summary readiness/refresh state. Those accepted parent-dashboard lines were not changed. Real wrapper integration was rendered in the marked dev page, not the full shared-data authenticated dashboard. Benchmark, ThaiJO/TCI, individual view, H-index and export paths were not modified.

## Synthetic cost and EXPLAIN

Scale: **5,246 eligible documents** (246 baseline + 5,000), 7,500 added country rows, five source IDs/seven source-year metric rows. Baseline had already undergone dirty-trigger tests. Only five synthetic authors, eight users and three catalogue affiliations are present; author/catalogue diversity/source-history depth do **not** model production. Relevant document/document-author/source-metric index shapes match `db/fund_cpkku.sql`. Duplicate metric rows were removed before adding the native unique index, after duplicate-join correctness was checked.

Five serial warm measurements per endpoint/filter. Controller snapshot/aggregation/JSON work includes harness response decoding/SQL trace overhead; excludes HTTP/auth/network/frontend rendering. Allocation is **TotalAlloc delta/request**, not peak/live memory. SQL durations include driver work, not a server-only timer. There is no concurrency/load/cold-cache or production SLA claim.

| Filter / endpoint | Docs | Median ms (min–max) | SQL statements | Allocated bytes/request | Response bytes |
|---|---:|---:|---:|---:|---:|
| All / summary | 5,246 | 606.45 (587.35–667.29) | 55 | 67,792,868 | 3,229 |
| All / drilldown, up to 200 | 5,246 | 625.75 (608.05–655.44) | 55 | 69,449,011 | 218,470 |
| BE 2569 + Q4 / summary | 5,239 | 667.20 (447.24–730.38) | 55 | 67,803,105 | 2,177 |
| BE 2569 + Q4 / drilldown, up to 200 | 5,239 | 611.04 (452.37–757.55) | 55 | 69,447,963 | 218,503 |
| Selective title / summary | 1 | 37.79 (29.19–47.54) | 16 | 188,078 | 1,877 |
| Selective title / drilldown | 1 | 83.60 (52.14–181.31) | 16 | 154,728 | 1,469 |

Saved **EXPLAIN FORMAT=JSON** plans use the exact production projection/base query under these three filters. They show author-link indexes, document PK lookup, unique source-year/type metric lookup and indexed metric subqueries; also duplicate removal, temporary table/filesort and scans/block nested-loop joins over tiny user/author/affiliation fixtures. Plan rows are optimizer estimates, not actual visits. Normalized year/name/ID expressions and substring title conditions remain in unchanged dashboard SQL. These plans do not establish production selectivity/latency.

Both endpoints still read the **complete filtered cohort** plus batched evidence for revision/dimensions/paging. Returning 200 rows does not bound snapshot work or memory. The observed ~0.61–0.67 s broad-filter medians and ~68–69 MB allocated/request at this scale warrant target resource/concurrent-load review before expanding scale. Future optimization must preserve snapshot, revision and unknown-evidence semantics.

## Changed files

Backend:

- `controllers/admin_scopus_faculty_insights_controller.go`: extracted the unchanged SQL projection for matching EXPLAIN.
- `controllers/admin_scopus_faculty_insights_integration_test.go`: native tag reuses synthetic helpers; valid native JSON seed preserves SQLite malformed-payload coverage.
- `controllers/admin_scopus_faculty_insights_mariadb_test.go`: guarded fixture/filter/paging, auth/session, snapshot/revision, optional UI handshake, cost/EXPLAIN and cleanup.
- This report; `docs/audits/faculty_insights_phase5_native_api.json`; `docs/audits/faculty_insights_phase5_runtime.json`; historical gate pointers in Phase 2 native-validation/Phase 3 reports.

Frontend:

- `AdminScopusFacultyInsights.js`: role/deduplication explanation.
- `app/dev/scopus-faculty-insights-integrated/{page.js,preview-client.js}`: guarded real-wrapper integration page.
- `middleware.js`: narrowly extends production 404 guard.
- `scripts/check-faculty-native-ui.mjs`: authenticated loopback-only QA; no saved JWT/auth headers.
- `docs/SCOPUS_FACULTY_INSIGHTS_PHASE5.md`; four PNGs, browser/production-guard JSON in `docs/faculty-insights-phase5-evidence/`.

## Reproduction

With the approved portable server running and empty local disposable schemas, from backend:

```powershell
$env:GOCACHE='G:\works-fund-project\.gocache'
$env:GOTMPDIR='G:\works-fund-project\.gotmp'
$env:CGO_ENABLED='0'
$env:SCOPUS_MARIADB_TEST_DSN='root:phase5-local-only@tcp(127.0.0.1:33117)/scopus_insights_test_phase2?parseTime=true'
go test -tags insight_mariadb ./services -run TestNativeMariaDBCoreInsights -count=1 -v -timeout 120s
$env:SCOPUS_MARIADB_API_TEST_DSN='root:phase5-local-only@tcp(127.0.0.1:33117)/scopus_insights_test_api?parseTime=true'
$env:SCOPUS_NATIVE_API_REPORT_PATH='G:\works-fund-project\fund-management-api\docs\audits\faculty_insights_phase5_native_api.json'
$env:SCOPUS_NATIVE_UI_HANDSHAKE_PATH='G:\works-fund-project\.phase5-mariadb\ui-handshake.json'
go test -tags insight_mariadb ./controllers -run TestNativeMariaDBFacultyInsights -count=1 -v -timeout 300s
```

Password above is synthetic. Select the exact native test: its tag reuses a fixture file containing SQLite tests too. Omit the handshake variable for native API-only execution; UI explicitly skips. With it set, the harness writes its current loopback API/JWT manifest and waits up to 240 s. Start frontend dev/browser QA in separate shells after the manifest exists, following frontend Phase 5 instructions. The harness removes old `.done` results to prevent stale success.

Regression commands actually run:

```powershell
# Backend, CGO_ENABLED=0
go test ./controllers ./models ./services ./routes ./cmd/scopus-core-insights -count=1
# Backend, GOARCH=386, CGO_ENABLED=1, CC=F:\MinGW\bin\gcc.exe
go test -tags insight_integration ./controllers ./services ./routes -count=1
# Frontend
node --test
node node_modules/next/dist/bin/next build
```

## Manager rollout sequence — prepared, not executed

1. Review/accept both repos and select the exact deployment DB/host. Preserve backups/migration history; verify document-ID signedness, engines/collations/indexes, trigger names and deploy-user FK/DDL/trigger privileges. Windows 10.11.16 success does not verify the configured Linux 10.11.18 target's exact schema/configuration.
2. Apply reviewed **`migrations/050_20261005_scopus_core_insights.sql`** to the explicitly authorized target **before deploying the ingest writer that requires its tables/guard**. MariaDB DDL auto-commits: do not promise transactional rollback. Verify four new tables, singleton guard, FK/indexes and dirty triggers. Missing migration deliberately returns 503; keep the UI unavailable until this gate passes.
3. Deploy accepted backend/API+writer. Absent/dirty/obsolete metadata remains unknown until normalized; GET never repairs it. First run bounded CLI **dry-run**, exact connected DB and fixed reviewed `MAX(id)` watermark. `--expect-database` must match configured and connected identity. Review unknown/conflict diagnostics and unchanged-cohort parity before persistence.
4. After target-write authorization, backfill **country evidence** in bounded batches, preserving successful `last_id` as exclusive `--after-id` and the same inclusive `--through-id`. Start e.g. `--limit 500 --batch-size 100`; review before scaling. The CLI makes no Scopus calls. Cached successful role checks remain the role source; this backfill does not fetch role XML or invent missing roles.
5. On 1213/1205/failure stop, preserve the successful cursor, correct causes and rerun a complete bounded document/transaction attempt. Never retry only a statement inside a failed transaction. Avoid concurrent bulk catalogue edits initially: guard/dirty triggers can serialize and deadlock. Replay dirty metadata after catalogue/raw changes.
6. Check target read-only summary/drilldown parity, permission denial, unknown partitions, years/metrics/filters, no-store headers, revisions and >200 paging. Earlier development audit counts apply only to identical cohort/filter/watermark inputs; no current production parity is claimed. Deploy/enable frontend after schema/backend/backfill health acceptance, then verify the full authenticated dashboard/applied filters and production QA 404s.
7. If reverting presentation/backend, retain additive schema/evidence for review. Do not automatically drop shared tables/triggers. Recheck older ingest compatibility and authorize cleanup separately.

Operator templates (not executed; replace reviewed placeholders):

```text
go run ./cmd/scopus-core-insights --expect-database REVIEWED_EXACT_DB --through-id FIXED_REVIEWED_MAX_ID --after-id LAST_SUCCESSFUL_ID --limit 500 --batch-size 100
# After dry-run review and target-write authorization:
go run ./cmd/scopus-core-insights --expect-database REVIEWED_EXACT_DB --through-id FIXED_REVIEWED_MAX_ID --after-id LAST_SUCCESSFUL_ID --limit 500 --batch-size 100 --apply
```

Local native/integration gates are closed. Shared migration/backfill, target privilege/performance verification, actual authenticated target dashboard validation and manager deployment acceptance remain rollout gates. No deployed-state/production-data claim is made.
