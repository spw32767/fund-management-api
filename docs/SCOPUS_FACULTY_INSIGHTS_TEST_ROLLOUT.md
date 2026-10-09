# Faculty research insights — authorized TEST database rollout

**READY FOR MANAGER REVIEW.** Authorized TEST migration/backfill completed on **5 October 2026, 06:29 Asia/Bangkok** (4 October 2026, 23:29 UTC). Actual TEST handler verification and final no-write reconciliation passed by **06:41:55 Bangkok** (23:41:55 UTC). This is a completed TEST data rollout, not a production deployment or live UI activation.

User explicitly authorized the TEST connection/rollout: “เชื่อมฐานข้อมูลทดสอบเลยครับ สรุปให้ผมด้วย”. This supersedes the earlier restriction on shared TEST migration/backfill. Production, deployment/push, Scopus harvesting and role XML changes remain excluded.

Baseline code: backend `457795f`, frontend `6036768`, both on `codex/faculty-research-insights`. Only backend opt-in verification/helper/report/evidence files are added; no product code, frontend configuration, branch, commits, deployment settings or running services were changed.

## Target, preflight and protected backup

- **Verified target:** development database `drnadech_fund_cpkku_intern`, connected MariaDB `10.11.18-MariaDB-0+deb12u1-log`. Environment/database and the Phase 1 endpoint fingerprint all match. No host, credentials, account identity, personal names/emails or raw payloads are included in reports.
- Fresh preflight: 949 core documents, fixed `MAX(id)=955`, compatible unsigned document ID/InnoDB tables, required DDL/trigger/data privileges, no existing migration-050 tables or triggers.
- No visible current writer. One import record remained `running` since **5 April 2026**, unchanged since that date; last saved API-request activity was **16 August 2026**. Other inspected Scopus run tables had zero running records. The stale record was left untouched. Process visibility is limited to this account; repeated source/activity checks are point-in-time evidence, not a guarantee against new external jobs.
- Before DDL, backed up affected/base table CREATE definitions, existing trigger definitions and any existing derived evidence. No source rows needed restoration because the rollout does not update them. Backup is **Windows DPAPI encrypted**, outside both repos in `G:\works-fund-project\.test-rollout-backup`; current-user encryption/decryption and recovery from disk were verified. Directory ACL was restricted to the creating Windows user. Recovery requires that same user/profile; it is not a portable plaintext dump. The private backup may contain schema definer identity and is never printed or committed.
- [Backup receipt](audits/faculty_insights_test_backup_receipt.json) contains the encrypted file hash and recovery scope. [Preflight](audits/faculty_insights_test_inspect.json) contains sanitized schema/privilege/activity/source-hash evidence.

## Applied migration and saved data

Migration 050 completed at **06:19:36 Bangkok** (23:19:36 UTC). Reviewed file SHA-256:

`498ec33503123623f64805e37d5b3dab076683ea226caab4c43b33f1e52e1180`

Created four additive tables, singleton guard `(1,0)` and seven missing reviewed triggers. The rollout helper intentionally skips DROP TRIGGER statements and preserves any matching existing trigger; **zero triggers were dropped**. Native document FKs/cascades, primary keys, InnoDB tables, guard and all seven actual trigger bodies were verified. No source/legacy/cohort tables were altered. [DDL receipt and resulting schema](audits/faculty_insights_test_migrate.json).

The existing reviewed CLI was compiled into private workspace temp. It normalizes saved Search JSON without making HTTP/Scopus requests. First ran **READ ONLY dry-run** in two bounded invocations (500 + 449), then authorized apply in **10 bounded invocations of at most 100**, SELECT batch size 50. Each invocation used exact `--expect-database drnadech_fund_cpkku_intern`, fixed inclusive `--through-id 955` and the previous successful exclusive `--after-id` cursor. Endpoint/identity, quiet activity and source fingerprints were checked before each invocation. There were **zero reported errors**, no automatic retries and final successful cursor **955**.

| Result | Dry-run | Persisted apply/reconciliation |
|---|---:|---:|
| Documents covered | 949 | 949 |
| International | 334 | 334 |
| Domestic | 610 | 610 |
| Unknown country | 5 | 5 |
| Country-complete | 941 | 941 |
| Country-incomplete | 8 | 8 |
| Normalizer version | core-countries-v2 | core-countries-v2 |

Persisted relation counts: **2,041 document/AFID rows**, **1,515 document/country rows**, **949 insight rows**, one guard row. **Zero duplicate document/country keys** and all 949 documents have insight metadata. Affiliation-complete is 942, country-complete 941: affiliation structure can be complete while its country remains unresolved.

The five unknown documents remain the same as Phase 1: 230 has unresolved affiliation/catalogue country; 302, 303, 503 and 779 have missing/malformed author AFIDs. Across all eight incomplete documents, that AFID reason occurs seven times; three incomplete documents still have positive current foreign evidence and correctly count international. No unknown was converted to domestic by assumption.

Source fingerprints are identical before/after DDL/backfill for saved raw payloads/document role status/check time/update time, legacy author links/role flags/affiliations, catalogue country mappings, cohort users and source metrics. No role XML was fetched or changed. [Dry-run](audits/faculty_insights_test_dry_run.json), [apply batches/cursors](audits/faculty_insights_test_apply.json), [saved reconciliation](audits/faculty_insights_test_verify.json).

## API verification and measured cost

**PASS — `TestConfiguredTestFacultyInsightsReadOnly`, 244.59 seconds.** The opt-in harness calls current summary/drilldown handlers directly in-process against the configured TEST DB. It does not initialize the full application, start an HTTP server or invoke production login/session refresh. Session `tx_read_only=1` is enforced on every audit connection, and handlers retain their own read-only repeatable-read transactions. Prior isolated Phase 5 native checks cover auth/permission middleware; target verification here covers actual saved data and handlers. No deployment/live authenticated UI pass is implied.

| Faculty scope | Unique works | International | Domestic | Country unknown | First | Corresponding | Co-author | Role unknown |
|---|---:|---:|---:|---:|---:|---:|---:|---:|
| All years | 743 | 251 | 489 | 3 | 145 | 200 | 398 | 0 |
| BE 2567–2569 | 226 | 122 | 104 | 0 | 20 | 82 | 124 | 0 |
| BE 2567 | 68 | 37 | 31 | 0 | 6 | 26 | 36 | 0 |
| BE 2568 | 81 | 35 | 46 | 0 | 9 | 31 | 41 | 0 |
| BE 2569 | 77 | 50 | 27 | 0 | 5 | 25 | 47 | 0 |

These match the earlier unchanged-cohort baseline. Corresponding excludes works already assigned faculty First; precedence prevents duplicate counting rather than ranking importance. Country/role percentages use the full matching population, including unknowns.

Passed actual-data checks:

- All 743 works across four server pages (200/200/200/143) and all 226 selected-year works across two pages (200/26), with unique IDs, matching totals/revisions and no first-200 truncation.
- **312 all-year + 48 selected-year cross-tab cells**, and **53 + 38 partner dimension counts**, each matching the corresponding real summary cell/count. Multiword country keys are URL-encoded correctly in the harness.
- `Cache-Control: no-store`, 64-character revisions, stale revision **409 without documents**, true zero-result/future-year response and unchanged revision after repeated reads.
- [Before/after read integrity](audits/faculty_insights_test_read_integrity.json): source fingerprints, all four derived-table fingerprints, revision and saved Scopus-request history remained unchanged. Audit connections were READ ONLY. The older running-job record was not modified.

[Sanitized handler summaries, dimensions and request costs](audits/faculty_insights_test_api.json) include no drilldown titles/author names/identifiers or connection identity. Full row data was inspected only transiently in the in-process responses and discarded.

Three serial warm samples per endpoint/filter on this actual **949-core / 743-faculty TEST dataset**, accessed over its configured network connection:

| Filter / endpoint | Matching faculty works | Median ms (min–max) | SQL statements/request | Response bytes |
|---|---:|---:|---:|---:|
| All years / summary | 743 | 530.87 (519.86–566.50) | 19 | 39,777 |
| All years / drilldown up to 200 | 743 | 545.08 (532.25–558.46) | 19 | 355,780 |
| BE 2567–2569 / summary | 226 | 428.21 (394.64–431.03) | 16 | 15,025 |
| BE 2567–2569 / drilldown up to 200 | 226 | 421.28 (369.13–444.78) | 16 | 366,331 |
| BE 2568–2569, Journal, T1/Q1–Q4 / summary | 104 | 372.87 (363.73–396.74) | 16 | 10,959 |
| Same Journal/quality filter / drilldown | 104 | 372.45 (369.64–380.17) | 16 | 194,406 |

Times include DB network/query/handler/JSON work and test SQL instrumentation/response decoding, but exclude HTTP transport, authentication and browser rendering. They are local serial warm TEST observations, not production latency/load/SLA claims. The whole filtered snapshot is still loaded before paging; page size bounds the returned rows, not all DB work. Saved SQL timings include driver work and are not server-only timers. No production settings/indexes were changed for these measurements.

## Local runtime and remaining UI steps

Read-only ownership inspection found **no local listener on configured port 8080** and no identified workspace-owned backend server. Port 8000 belongs to an unrelated Python process and was preserved. No process was killed/restarted; no externally reachable service was launched. [Sanitized runtime inspection](audits/faculty_insights_test_runtime.json).

The TEST database is migrated and normalized. Showing this data through a running UI still requires the serving backend/frontend to use the accepted code and intended TEST API origin. That runtime activation was not performed or claimed. `cmd/api` currently binds `:8080` and starts background jobs including MOU notification delivery; starting it directly is not a reviewed loopback-only action. Manager can separately review a development launcher that binds only loopback and controls background side effects, or coordinate the actual serving backend's owner. Preserve unrelated sessions; do not redeploy production or change production configuration as part of this rollout.

The old Phase 5 report describes its pre-rollout state; this report supersedes its “shared TEST migration/backfill unapplied” statement. Production remains untouched.

## Review and reproduction

Added opt-in [TEST rollout helper](audits/faculty_insights_test_rollout.py) and [TEST handler verification](../controllers/admin_scopus_faculty_insights_test_rollout_test.go), plus this report/sanitized evidence. Helper rejects a different environment/database/endpoint fingerprint. Backup is private outside Git; no credentials/data dump enters evidence. It checks the backup hash/source/schema stability before DDL and does not gratuitously recreate triggers.

Python helper compilation and both repository whitespace/status checks completed; frontend is clean and all new backend artifacts remain uncommitted for manager review. No further general regression rerun was needed because no product code changed; the new tagged helper compiled/skipped safely without its opt-in flag, then passed the actual-data run. Earlier approved product/native regression gates remain recorded in Phase 5.

Automatic approval review initially failed on a local ownership-inspection call due to the account usage limit; that call was not executed. After the user's continue request, the read-only ownership inspection succeeded. The already-approved bounded backfill continued and completed normally; no approval workaround was used.

Read-only reproduction from backend root:

```powershell
python docs/audits/faculty_insights_test_rollout.py inspect
python docs/audits/faculty_insights_test_rollout.py verify
$env:GOCACHE='G:\works-fund-project\.gocache'
$env:GOTMPDIR='G:\works-fund-project\.gotmp'
$env:CGO_ENABLED='0'
$env:SCOPUS_TEST_ROLLOUT_VERIFY='authorized-test-read-only'
$env:SCOPUS_TEST_ROLLOUT_REPORT='G:\works-fund-project\fund-management-api\docs\audits\faculty_insights_test_api.json'
go test -tags insight_test_rollout ./controllers -run TestConfiguredTestFacultyInsightsReadOnly -count=1 -v -timeout 15m
```

Without the opt-in flag, the tagged harness explicitly skips configured DB access; untagged tests do not include it. Do not rerun backup/migration/apply merely to reproduce read results. Any future bounded resume should use the reviewed CLI's exact target/fixed watermark and successful cursor after fresh inspection. Rollout helper reports database errors without connection identity/source data. Initial helper probes corrected native column/grant syntax assumptions; initial API probes corrected session-variable interpretation and multiword country query encoding. No production behavior changes were needed.
