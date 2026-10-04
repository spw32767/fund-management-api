# Phase 2 native validation revision

**READY FOR MANAGER REVIEW. Native MariaDB validation remains NOT RUN.** This revision completes the explicitly authorized fallback: runtime discovery, production fixes, and a runnable guarded native integration harness. No system software was installed, no databases were created on the configured remote server, no shared DB writes were issued, and no commits/branch switches/API/UI work were performed.

## Runtime evidence

[Sanitized runtime inventory](audits/faculty_insights_phase2_runtime_inventory.json) records the checks. Docker, Podman, mysqld, mariadbd, mariadb commands were absent; common Docker/Podman/MySQL Server/MariaDB/XAMPP executable locations were absent; no matching database/container service was found. `G:\MySQL` contains Workbench support files, not a server.

WSL initially returned `Wsl/Service/E_ACCESSDENIED` inside the sandbox. An approved **read-only** inventory call succeeded and reported **no installed distributions**, `Wsl/WSL_E_DEFAULT_DISTRO_NOT_FOUND`, default WSL version 2. `WSLService` runs; `WslInstaller` is stopped. Therefore there is no discovered suitable existing native runtime. Neither WSL distributions nor Docker/database software was installed as a workaround.

`go test -tags insight_mariadb ./services -run TestNativeMariaDBCoreInsights -count=1 -v` compiled the harness and explicitly emitted:

`NATIVE MARIADB NOT RUN: SCOPUS_MARIADB_TEST_DSN is unset` / `SKIP`.

The command's exit success means compilation and skip handling worked; **it is not a native test pass**. Actual MariaDB migration syntax, FK/trigger execution, lock/gap behavior and deadlock results remain unverified until the harness runs on an approved isolated runtime.

## Production revisions

1. **Catalogue concurrency, including missing/padded AFIDs:** migration 050 adds a singleton `scopus_country_catalogue_guard` and BEFORE insert/update/delete catalogue triggers which lock/advance it atomically. Write normalization takes a SHARE lock on that row before its catalogue reads and persistence. This closes the race where an unmapped AFID is inserted between normalization and relation publication, including alternate padded spellings that do not occupy the same unique-index gap. Existing AFTER triggers mark all canonical dependent documents dirty. Locks/revision updates roll back with the mutation. Dry-run does not require or touch this new table.
2. **Collation:** normalizer version is now `core-countries-v2`. Document and author AFIDs and supplied catalogue keys use lowercase/trimmed canonical keys; equivalent duplicate IDs deduplicate. Conflicting catalogue rows that collapse to one canonical key become unresolved rather than picking an arbitrary mapping. New AFID/country relation keys explicitly use `utf8mb4_bin`. MariaDB catalogue lookup uses `BINARY LOWER(TRIM(afid))`, preventing an accent-insensitive legacy collation from inventing `a`→`á` mappings. Triggers use the same lowercase/ASCII-padding normalization and byte-sensitive change detection (`BINARY OLD... <=> BINARY NEW...`), including collation-equivalent renames. Scopus AFIDs in the audited dataset are numeric; synthetic case/accent fixtures protect broader data/import behavior.
3. **Raw payload writers:** source searches found only `ScopusIngestService.processEntry` assigning core `RawJSON` and saving core documents in runtime code. Role/conference services update selected status/conference fields only; benchmark, metrics and ThaiJO raw JSON belong to separate tables. `db/fund_cpkku.sql` includes historical core INSERTs. Migration 050 now adds an AFTER core-document update trigger: a byte-changed `raw_json` clears international/completeness and sets `dirty_payload`, covering direct SQL/import and future writers too. Missing metadata after inserts is also unclassified. Phase 3 can gate solely on metadata status/version, without parsing raw JSON per dashboard request. Its cache key/invalidation must observe dirty transitions and later repaired checks, alongside other cohort/role/metric dependencies.
4. **Dirty-state replay:** the hash shortcut only applies to `complete` or `incomplete`. Changing a payload and then restoring the old bytes still forces repair from `dirty_payload`; it cannot reuse an old hash and remain stale. Stale country rows are ignored until repair, including while dirty catalogue/raw states are present.
5. **Abort/retry behavior:** existing ingest receives `processEntry` errors, records `DocumentsFailed`, and continues; it does not automatically retry that document. Backfill aborts with the last successful cursor. Its CLI identifies MySQL 1213/1205 as a sanitized retryable lock conflict and recommends rerunning from `last_id`. `processEntry` now restores pre-transaction counters on failure so a rolled-back attempt does not inflate committed creation/update counts.

The singleton guard is intentionally conservative and can serialize catalogue mutation. Function-normalized catalogue lookup can scan the small catalogue (450 rows in the audit). This is a correctness-first write path; it is not used on each dashboard request. Lock order remains capable of deadlocks: document locks, catalogue row writes, guard locks and multi-document dirty updates can intersect. Failures must abort the **whole transaction**, never retry a failed statement inside it. On 1213/1205 retry at most a few complete attempts with backoff, rereading document/payload/catalogue; stop/report persistent conflicts. The current CLI supports explicit operator rerun/resume rather than an implicit loop, and failed ingest documents can be replayed through the existing author import workflow. No new Scopus calls were made during this revision. The native harness exercises a forced lock cycle and verifies the victim restores raw payload, derived flags and memberships together; that test is prepared, not executed here.

## Native harness and commands

[services/scopus_insight_mariadb_test.go](../services/scopus_insight_mariadb_test.go) is tagged `insight_mariadb`. It **never reads `.env`/DB_* configuration**. It accepts only TCP loopback, an exact connected schema named `scopus_insights_test_*`, actual MariaDB 10.11, and an initially empty schema. The caller provisions that isolated schema. The test does not create a database. It creates synthetic fixture tables after the guards and removes only its fixed owned table names afterward.

Coverage prepared: actual migration 050 and repeated application before/after data; audited unsigned document IDs, JSON raw payloads, legacy case-insensitive AFIDs; FK rejection and cascades; catalogue insert/update/delete/AFID rename; previously missing padded AFID insertion; binary accent separation; complete→partial/full replacement and replay; transaction rollback; raw-payload update/restore; concurrent normalization vs catalogue update/missing insertion; forced catalogue/normalization deadlock rollback; executed GORM `LOCK IN SHARE MODE` SQL capture.

On a machine **already equipped with an approved Docker runtime**, an example disposable fixture is:

```powershell
docker run --rm -d --name scopus-insights-mariadb -p 127.0.0.1:3307:3306 -e MARIADB_ROOT_PASSWORD=local-test-only -e MARIADB_ROOT_HOST=% -e MARIADB_DATABASE=scopus_insights_test_phase2 mariadb:10.11 --character-set-server=utf8mb4 --collation-server=utf8mb4_unicode_ci
```

After that container is ready, from the backend root:

```powershell
$env:SCOPUS_MARIADB_TEST_DSN='root:local-test-only@tcp(127.0.0.1:3307)/scopus_insights_test_phase2?parseTime=true'
$env:CGO_ENABLED='0'
go test -tags insight_mariadb ./services -run TestNativeMariaDBCoreInsights -count=1 -v -timeout 120s
Remove-Item Env:SCOPUS_MARIADB_TEST_DSN
docker stop scopus-insights-mariadb
```

The password above is a synthetic fixture value, not an application credential. These runtime provisioning commands were **not executed** here. Use a disposable new schema/container for each run; the harness rejects nonempty schemas and remote/shared configuration. A failed partial fixture setup should be discarded by stopping that disposable container rather than bypassing its guard.

## Checks actually run

- Backend regression: `go test ./controllers ./models ./services ./cmd/scopus-core-insights -count=1`, CGO disabled — PASS.
- Pure AFID casing/padding/ambiguous-catalogue/accent tests and sanitized 1213/1205 retry-hint tests — PASS.
- GORM initialization with scripted `SELECT VERSION() = 10.11.18-MariaDB` and actual driver clause generation — PASS; generated `LOCK IN SHARE MODE`, not `FOR SHARE`. This proves dialect selection, not execution by a live server.
- Isolated SQLite `insight_integration` tests, installed 32-bit GCC/GOARCH=386 — PASS, including new raw-update/restore freshness and rollback counter checks. SQLite does not validate native guard locks or MariaDB trigger syntax.
- Refreshed **READ ONLY** v2 development dry-run: **949 = 334 yes / 610 no / 5 unknown**, 941 country-complete, watermark 955; selected unchanged faculty cohort: **226 = 122 yes / 104 no / 0 unknown**. [Core results](audits/faculty_insights_phase2_dry_run.json), [faculty parity](audits/faculty_insights_phase2_faculty_parity.json). Baseline counts are unchanged.

The isolated MariaDB deployment gate remains pending. Manager can inspect/commit the revision or provide an approved native runtime for execution; this chat does not proceed to Phase 3 or apply migration/backfill to the shared development database.
