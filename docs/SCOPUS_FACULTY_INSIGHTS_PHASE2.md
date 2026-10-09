# Faculty research insights — Phase 2 data foundation

**READY FOR MANAGER REVIEW.** Implemented Phase 2 only on `codex/faculty-research-insights`, after accepted Phase 1 commit `d8618b2`. No commits, pushes, branch changes, subagents, Scopus requests, API/UI changes, or migrations/backfill application against the configured shared database. The frontend remains unchanged. The current dashboard cohort and legacy eligible-author rules are preserved.

**Manager-review revision:** [Native validation report](SCOPUS_FACULTY_INSIGHTS_MARIADB_VALIDATION.md) supersedes initial locking/collation/raw-writer details below. Version is now `core-countries-v2`; migration 050 additionally contains a catalogue guard table/BEFORE triggers and raw-payload dirty trigger. Backend/SQLite checks and read-only baselines pass again. The guarded native harness compiles but was explicitly **SKIPPED**, because runtime discovery found no installed WSL distribution/container/database server. Actual native validation is still pending; no native pass is claimed.

## Implementation and evidence policy

- `migrations/050_20261005_scopus_core_insights.sql`: additive core document-affiliation, document-country, and document-insight tables. Unmapped AFIDs are retained without a catalogue FK. Country memberships include Thailand and are unique per document/canonical key. Document FKs cascade on deletion; reverse AFID/country indexes support catalogue invalidation and future country drilldown.
- `models/scopus_insight.go`: separate metadata prevents generic `ScopusDocument.Save` calls from resetting derived checks. Nullable international flag, separate affiliation/country completeness, status/reasons/diagnostics, provenance, normalizer version, payload/catalogue hashes, and check time are persisted.
- `services/scopus_insight_normalizer.go`: shared deterministic pure normalizer for saved Search payload and online ingest. Parses object/array structures, numeric/string `$` scalars including Search `@limit`/`@_fa` attributes, every author AFID and top-level AFID, and duplicates. Requires a matching positive author-count, nonempty valid author/document affiliation lists, unique/nonempty author IDs, every supplied author having AFIDs, and every author AFID appearing in the document list before affiliation completeness. Explicit truncation, malformed structure and missing fields defeat completeness.
- `services/scopus_insight_service.go`: bounded per-document persistence and backfill, document locking/re-read, catalogue guard/relevant catalogue share locks, atomic relation replacement plus metadata upsert, idempotence through hashes/version and valid status, and resume cursor. No HTTP clients or harvesting calls.
- `services/scopus_ingest_service.go`: locks an existing document, synchronizes country evidence inside its existing transaction, preserves original XML role invalidation, and preserves an existing catalogue country when a Search payload omits/empties the country. Explicit catalogue edits can clear mappings.
- `cmd/scopus-core-insights`: dry-run by default, required exact `--expect-database` checked against both configuration and connected database, explicit inclusive `--through-id`, bounded limit/batch, exclusive `--after-id` resume, opt-in `--apply`. Uses a `REPEATABLE READ`, `READ ONLY` transaction for dry-run and rolls it back. Dry-run works before migration 050 exists. SQL/connection errors are sanitized and GORM logging is silent, avoiding credentials or raw payload output.

An all-author affiliation table is intentionally omitted: country classification, country drilldowns and catalogue invalidation only require the document union. The existing first-affiliation author links remain the cohort/role source; raw JSON retains full author-to-AFID evidence. Adding another relation now would duplicate synchronization without a Phase 2 consumer. A future explicitly assigned cohort change can introduce it if needed.

**Replacement policy:** each saved/current payload replaces the previous derived document relations, even when partial. No silent union with old payloads and no inherited domestic assertion. Consequently a partial payload may lose previously known foreign evidence and become unknown; this is deliberate because the old evidence would otherwise be presented as current. Known foreign evidence in the current data establishes `yes` even if incomplete. `no` requires complete resolved Thailand-only evidence. Everything else is `NULL`/unknown. Known country evidence on an affiliation lacking an AFID can still establish foreign presence, while completeness remains false.

**Country validation and conflict policy:** the versioned registry recognizes the 54 observed source spellings plus reviewed common labels/aliases, including `Viet Nam`→`Vietnam`, `Russian Federation`→`Russia`, `Macau`→`Macao`, `Türkiye`→`Turkey`, and case/whitespace variants. It is a conservative curated registry, not an exhaustive geographic authority; unlisted names/placeholders do not establish foreign presence and are reported unresolved. Extending it requires reviewed labels and a version bump/replay.

For a referenced AFID with a catalogue row, the catalogue country is authoritative, including an explicitly cleared/unrecognized value. A conflicting payload country is retained in `payload_country`, recorded as a reason, and blocks full country completeness. A recognized authoritative foreign country still proves `yes`; a catalogue Thailand / payload foreign conflict becomes unknown rather than certain domestic. With no catalogue row, recognized payload country evidence is usable and the AFID is retained. Conflicting duplicate payload countries block completeness; known foreign presence still proves `yes`. Both original payload spelling and evidence source remain auditable.

**Catalogue synchronization:** migration 050 includes single-statement insert/update/delete triggers on `scopus_affiliations`. In the same catalogue mutation transaction, every dependent metadata row becomes `dirty_catalogue`, its international flag becomes NULL and `countries_complete` becomes false. This also covers insertion of previously unmapped AFIDs and AFID renaming/deletion. The next ingest or backfill recomputes from the current payload/catalogue. Old country memberships remain stored until recomputation as stale evidence; **Phase 3 must ignore country rows whenever metadata is dirty, absent, or on an obsolete normalizer version**, or recompute before use. No stale domestic assertion remains in metadata. Unchanged country/AFID updates do not dirty results. The relevant-catalogue hash and locks additionally protect application recomputation against concurrent catalogue changes; database deadlocks abort rather than publish partial state and can be retried by the operator.

No faculty-role classification is added in this phase. Phase 3 must use a successful current document role status before trusting a positive First flag; stale flags on pending/needs_review/fetch_error must not be copied from the audit helper as production logic. A trustworthy First can beat another eligible author's unknown role; otherwise unknown blocks Corresponding/Co-author.

## Validation results

Completed 5 October 2026, **01:27 Asia/Bangkok** (4 October 2026, 18:27 UTC). Read-only runs used the same configured **development**, not verified production, database as Phase 1. No migration or apply command was issued against it.

| Check | Result |
|---|---|
| `go test ./controllers ./models ./services ./cmd/scopus-core-insights -count=1` with CGO disabled | PASS; controllers, services and CLI tests; models compile |
| `go test -tags insight_integration ./services -run TestCoreInsight -count=1` with GOARCH=386, CGO_ENABLED=1, installed `F:\MinGW\bin\gcc.exe` | PASS; isolated in-memory SQLite |
| Pure normalizer fixtures | PASS: 27 shape/completeness/conflict cases plus alias/deduplication/hash test |
| CLI exact target guard | PASS: missing/mismatched/space-altered targets rejected for dry-run and apply |
| Whole-core CLI dry-run | 949 processed through ID 955: **334 yes / 610 no / 5 unknown**, **941 country-complete** |
| Production-normalizer faculty parity audit, unchanged dashboard SQL, CE 2024–2026 | **226 total / 122 yes / 104 no / 0 unknown** |

The first live dry-run identified Search scalar wrapper attributes that were absent from initial fixtures; those wrappers originally caused author-count/AFID completeness failures. The parser now accepts `$` plus `@` attributes, a regression fixture covers the real forms, and the final counts match Phase 1 without changing classification rules or tuning to totals.

Integration tests cover domestic→foreign→partial→full replacement, deduplicated memberships, idempotent check time, metadata failure rolling back relations, country insert/update/clear/delete invalidating multiple dependent documents, authoritative catalogue conflicts, preservation of reliable catalogue country on partial Search payload, ingest role preservation and changed-roster flag clearing, entire failed ingest rollback, noncontiguous ID cursors, dry-run no writes, replay, and invalid bounds.

Test dependencies `gorm.io/driver/sqlite` / `github.com/mattn/go-sqlite3` are already cached dependencies added to go.mod/go.sum. Integration tests are behind `insight_integration`, so normal builds/tests do not require CGO. The installed GCC is 32-bit, hence GOARCH=386 for that isolated test run. No local MariaDB server binary was available; SQLite tests use equivalent catalogue triggers and do **not** validate actual MariaDB trigger syntax, FK DDL, or locking/deadlock behavior. Those checks remain a deployment gate for an isolated MariaDB staging instance. No product UI/build work belongs to this phase.

Sanitized artifacts:

- [Whole-core final dry-run](audits/faculty_insights_phase2_dry_run.json).
- [Selected faculty parity](audits/faculty_insights_phase2_faculty_parity.json).
- [Read-only parity helper](audits/faculty_insights_phase2.go), using the production normalizer and the exact unchanged dashboard gate, bounded to 100,000 rows and a read-only transaction. It exports no source JSON/credentials.

## Migration and backfill plan — prepared, not executed

1. Review/apply 050 in an **isolated MariaDB** instance with representative core tables first. Verify table FK compatibility against real `scopus_documents.id`, trigger privileges and all catalogue events, atomic failures and concurrent ingest/backfill/catalogue corrections, rerunning the migration, and replay idempotence. The prepared migration uses single-statement triggers, requiring no client-specific `DELIMITER` handling.
2. After manager approval, quiesce ingest and catalogue writers during the shared development rollout. Apply migration 050 **before** deploying the ingest binary; the new ingest path requires the tables. Rerunning the migration drops/recreates triggers, so writer quiescence prevents an invalidation gap. It does not harvest or backfill any data by itself.
3. Capture a fresh core maximum ID and run the same bounded dry-run. The audited maximum was **955**, with gaps and 949 documents. Fixed watermark plus cursor prevents unbounded drift. Example from backend root:

   `go run ./cmd/scopus-core-insights --expect-database drnadech_fund_cpkku_intern --through-id 955 --limit 100000 --batch-size 100`

   This is the exact successful default READ ONLY dry-run invocation; no `--apply` was used. When authorized after staging/migration review, add `--apply` to persist. For shorter jobs use `--limit 100` and resume with `--after-id <last_id>` while retaining the same watermark. Each document commits atomically; returned `last_id` is the last successfully completed document, so a failed invocation resumes after it. Replaying from zero is safe and repairs dirty catalogue state. To repair earlier dirty documents, replay the full bounded range rather than continuing only beyond the old cursor.
4. Reconcile all 949 core classifications with the accepted baseline, selected faculty totals with 226/122/104, unresolved IDs/reasons with Phase 1, and count/index integrity. Catalogue revisions/dirty statuses and country rows must be considered together. Restore ingest only after verification. Normalization does not alter the existing dashboard cohort or XML role job.
5. Phase 3 must supply safe cache/revision behavior for country/catalogue, raw-payload, role, user-cohort and metric changes. No new endpoint/cache is shipped here; the metadata hashes/check state are the data foundation, not a completed multi-instance API invalidation system.

## Handoff

Changed: new model, migration 050, shared normalizer/persistence service, bounded CLI/tests, normalizer/unit and tagged integration tests, ingest synchronization/country preservation, SQLite test dependencies, this report and three Phase 2 audit artifacts. Phase 1 files are unchanged; frontend is clean. All targeted regression/integration checks pass and live read-only baselines reconcile.

Validation revision additionally changes the same migration/normalizer/service/ingest/CLI, adds the native opt-in harness, GORM dialect test, runtime inventory and native validation report, and adds raw-writer/collation/counter regression coverage. **Native MariaDB execution remains pending.**

**READY FOR MANAGER REVIEW.** Manager owns acceptance/commit and authorization of shared development migration/backfill. Await further assignment; do not proceed to API/UI phases.
