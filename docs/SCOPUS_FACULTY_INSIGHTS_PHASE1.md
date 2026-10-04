# Faculty research insights — Phase 1 audit

**READY FOR MANAGER REVIEW. Phase 1 only; no product changes, migrations, DB writes, API harvesting, commits, pushes, branch switches, subagents, or automations.**

## Snapshot, scope, and reproducibility

Audit completed **5 October 2026, 01:05:25 Asia/Bangkok** (4 October 2026, 18:05:25 UTC). Backend and frontend remain on `codex/faculty-research-insights`. Both repos were clean at entry. No applicable `AGENTS.md` was found in the workspace/repo trees or `G:\` ancestry; the workspace `.agents` directory was empty.

This is the **configured development database**, not a verified production snapshot: `ENVIRONMENT=development`, database `drnadech_fund_cpkku_intern`, non-loopback endpoint fingerprint `473f44f70b3c`, MariaDB `10.11.18-MariaDB-0+deb12u1-log`. Credentials, account identity, host/address, and raw payloads are omitted. `.env` is read internally, with existing process environment taking precedence. A sandboxed connection initially failed with PyMySQL `OperationalError` 2003; the approved network-enabled read-only run succeeded.

Evidence files:

- [Audit helper](audits/faculty_insights_phase1.py): SELECT-only data reads, bounded queries, explicit session `REPEATABLE READ` and `READ ONLY`, consistent snapshot, unconditional rollback/close. It does not import application initialization or call Scopus. Requires already available `pymysql` and `python-dotenv`.
- [Sanitized audit results](audits/faculty_insights_phase1_results.json): SQL text under `queries`, live schema/index inventory, aggregates, bounded publication/author-ID evidence. Absent counter keys mean zero. No source JSON, personal names, emails, or connection secrets are exported.
- Reproduce from backend root: `python docs/audits/faculty_insights_phase1.py > docs/audits/faculty_insights_phase1_results.json`. Network permission may be needed. Do **not** use `cmd/test-db`.

Final run verified `@@session.tx_read_only=1`. It read all **949 documents**, **3,734 author links**, and **450 affiliation catalogue rows**, below caps of 100,000 documents/catalogue rows and 1,000,000 links. Connection timeout is 10 seconds; read timeout is 45 seconds. The audit issues no DDL/DML and ends with `ROLLBACK`. A read-only transaction does not prove production identity; the counts below describe this local configuration only.

## Findings and baseline

All counts use unique `scopus_documents.id`; the unique EID constraint prevents duplicate publication EIDs. “Faculty” below means the **existing dashboard cohort**, without quality/type/text filters. BE 2567–2569 means calendar CE 2024–2026.

| Measure | All core | Dashboard faculty, all years | Faculty BE 2567–2569 |
|---|---:|---:|---:|
| Unique documents | 949 | 743 | 226 |
| Raw JSON valid object / nonempty document affiliations / matching author-count | 949 / 949 / 949 | 743 / 743 / 743 | 226 / 226 / 226 |
| Operationally complete affiliation metadata | 941 | 740 | 226 |
| Incomplete affiliation metadata | 8 | 3 | 0 |
| International: any known foreign country | 334 | 251 | 122 |
| Domestic: complete, Thailand only | 610 | 489 | 104 |
| Country status unknown | 5 | 3 | 0 |
| Legacy first-affiliation international | 326 | 243 | 119 |
| International documents missed by legacy | 8 | 8 | 3 |
| Documents / author rows with multiple AFIDs | 108 / 165 | 95 / 142 | 44 / 72 |
| Documents / author rows with Thailand + foreign dual affiliations | 34 / 36 | 33 / 35 | 6 / 7 |

No raw JSON is missing, malformed, or a non-object. No declared author-count differs from the supplied author array, no stored/payload author-ID roster mismatch was observed, and no referenced author AFID is absent from the document affiliation list. There are **14 author rows without AFIDs** across the eight incomplete documents, three in the dashboard cohort. The snapshot contains no top-level affiliation-count or independent completeness/truncation assertion.

The separate **all-core BE 2567–2569** subgroup has **243** documents: **127 international, 116 domestic, zero country-unknown**, all operationally complete; legacy detection finds 124 international. Of these, 226 meet the dashboard faculty gate and 17 do not. Its XML role statuses are 215 complete / 28 no_correspondence, with zero NULL link flags. The results retain this subgroup independently from the faculty population.

**Completeness is an operational inference, not proof of exhaustive source metadata.** The audit requires a nonempty document affiliation list; valid author-count matching a nonempty roster; nonempty AFIDs for every supplied author; every author AFID represented in the document affiliation list; every document affiliation having an AFID and resolvable country. It cannot detect countries/affiliations silently omitted by Scopus when those checks still pass. A missing count, malformed shape, or explicit truncation must remain incomplete in future normalization. Positive foreign evidence establishes `yes` even when metadata is incomplete; incomplete Thailand-only evidence remains `unknown`.

| Calendar BE | All filtered Scopus works | International | Domestic | Unknown country | International / all | First | Corresponding | Co-author | Unknown role |
|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|
| 2567 | 68 | 37 | 31 | 0 | 54.41% | 6 | 26 | 36 | 0 |
| 2568 | 81 | 35 | 46 | 0 | 43.21% | 9 | 31 | 41 | 0 |
| 2569 | 77 | 50 | 27 | 0 | 64.94% | 5 | 25 | 47 | 0 |
| Total | 226 | 122 | 104 | 0 | 53.98% | 20 | 82 | 124 | 0 |

All-year faculty international percentage is 251/743 = **33.78%**, including the three unknown-country works in the denominator. Totals are weighted from document counts, never an average of yearly percentages.

### Real affiliation and country evidence

- Document **39**, EID `2-s2.0-85046824606`, author `57371148100`: AFIDs `60280609` (Thailand), `60112696` (Viet Nam). Legacy links show Thailand only; payload countries show both. Document **45**, EID `2-s2.0-85044056513`, has the same dual-affiliation pattern. This meets the agreed foreign operational definition even when the same author has both affiliations.
- Document **30**, EID `2-s2.0-85138626819`, author `56562406000`: `60017165` (Thailand) and `60008734` (Netherlands). Document **289**, EID `2-s2.0-85196849176` (CE 2024), author `59188288200`: `60026046` (Thailand) and `60367363` (Indonesia).
- All eight missed international document IDs are **39, 45, 410, 414, 721, 802, 912, 953**. In the selected years they are **912** (`2-s2.0-105033664816`, 2025, Viet Nam), **721** (`2-s2.0-105015041582`, 2026, India), and **953** (`2-s2.0-105035320416`, 2026, United States). Results include full EIDs and country evidence.
- No AFID lacks a catalogue row. **AFID `126187062` has no country**, affecting document **230**, EID `2-s2.0-85104665755`; payload also supplies no usable country. This document is outside the current dashboard cohort and is country-unknown.
- Current faculty country-unknown document IDs are **302** (`2-s2.0-85083259138`), **303** (`2-s2.0-85078284899`), **503** (`2-s2.0-84866081039`). Each has Thailand evidence but a supplied author without AFIDs. All-core unknown also includes **230** and **779** (`2-s2.0-85098514915`). None is in CE 2024–2026. Three other incomplete works have known foreign evidence and therefore remain `yes`.
- **54 nonempty country spellings** occur in payloads. Catalogue spellings match payload spellings where both resolve: no normalized country conflict observed. Existing spellings include `Viet Nam`, `South Korea`, `Russian Federation`, `Macao`, `Taiwan`, `United States`, `United Kingdom`, `Turkey`. Whitespace collapse and case folding suffice for this snapshot; no observed alias pairs need merging. Future explicit aliases should retain original spelling/provenance and use a versioned canonical key (e.g. `Viet Nam`/`Vietnam`), rather than treating every unfamiliar nonempty string as a verified country.
- Store **Thailand too**. For display, exclude Thailand from partners. Selected-year partner counts sum to **294** for **122** unique international works; the all-year faculty partner sum is **527** for **251** works. Multiple countries per work explain the difference. Selected-year leaders: India 73, Saudi Arabia 28, Malaysia 19, Japan 16, South Korea 15, Viet Nam 14, United States 14. These are unique documents per country, not author/AFID occurrences.

### Author-role evidence and precedence

Live core role status: **671 complete, 278 no_correspondence, zero pending/fetch_error/needs_review**. Faculty subset: **520 complete, 223 no_correspondence**; selected years: **200 complete, 26 no_correspondence**. All 3,734 stored author links have both role flags non-NULL. The historical 949 snapshot in [SCOPUS_AUTHOR_ROLES.md](SCOPUS_AUTHOR_ROLES.md) happens to match this audit, but was not used as the count source.

For all core author links: first=949, corresponding=743, both=285, neither=2,327. For eligible faculty author links: **983 links**, first=145, corresponding=286, both=64, neither=616. For selected years: **307 links**, first=20, corresponding=94, both=6, neither=199. These overlapping link counts cannot be used directly as document donut slices.

| Document-level faculty roles | All-year faculty | Faculty BE 2567–2569 |
|---|---:|---:|
| Any faculty First flag | 145 | 20 |
| Any faculty Corresponding flag | 285 | 94 |
| Any faculty Co-author (both flags zero) | 502 | 163 |
| First + Corresponding overlap | 85 | 12 |
| First + Corresponding + Co-author overlap | 32 | 8 |
| Same eligible faculty author is First + Corresponding | 64 | 6 |
| Exclusive First / Corresponding / Co-author / Unknown | 145 / 200 / 398 / 0 | 20 / 82 / 124 / 0 |

Document **3**, EID `2-s2.0-105004911616` (2025): eligible author `54683571200` has `(first=1, corresponding=1)`; `35179942700` and `57218454758` each have `(0,0)`. The faculty document counts **once as First**, despite all three positive role categories existing. Selected-year exclusive donut percentages are **8.85% / 36.28% / 54.87% / 0%**.

Recommended classification across eligible faculty authors: any positive First => First; otherwise any unresolved eligible role/status => Unknown; otherwise any Corresponding => Corresponding; otherwise all confirmed non-first/non-corresponding => Co-author. No eligible author => Unknown. `author_seq=1` alone must not override XML role evidence. NULL must never become zero. An unknown author must block premature Corresponding/Co-author assignment, including when another eligible author has a positive Corresponding flag. A known First still wins over another author's unknown role.

Current faculty role-unknown count is zero. The `all_core.faculty_role_unknown=206` diagnostic is **outside-cohort documents with no eligible dashboard faculty author**, not 206 incomplete XML records. `no_correspondence` follows the already agreed XML operational convention: non-first authors without identified correspondence are co-authors; it does not assert publishers have no corresponding authors. Co-first is not supported by current flags.

| Country status | First | Corresponding | Co-author | Unknown role | Total (selected years) |
|---|---:|---:|---:|---:|---:|
| International (`yes`) | 6 | 30 | 86 | 0 | 122 |
| Domestic (`no`) | 14 | 52 | 38 | 0 | 104 |
| Unknown country | 0 | 0 | 0 | 0 | 0 |
| Total | 20 | 82 | 124 | 0 | 226 |

International role percentages: **4.92% / 24.59% / 70.49%**; domestic: **13.46% / 50.00% / 36.54%**. All-year cross-tab is `yes=(34,78,139,0)`, `no=(111,122,256,0)`, `unknown=(0,0,3,0)`. Unknown-country works belong in the cross-tab, excluded from both comparison donuts.

## Exact current cohort, filters, and benchmark differences

Code references are repository-relative and refer to the unchanged audited code:

- `controllers/admin_scopus_dashboard_controller.go:306–327`, `applyScopusKKUAffiliationConstraint`: document must have an author whose `scopus_author_id = TRIM(users.scopus_id)`, with nondeleted, nontest user and nonempty ID, and that link's **legacy first affiliation name**, lowercased/trimmed, exactly `khon kaen university` or `faculty of science, khon kaen university`. It does **not** require `role_id IN (1,4,5)`, employment date, department, active status, or AFID membership. `scope=faculty/individual` mainly controls extra individual aggregates; both use this document constraint.
- Faculty roster and teacher total (`:872–882`, `:1224–1234`) separately require `delete_at IS NULL`, `is_test=0`, `role_id IN (1,4,5)`. Live data: **53** nondeleted/nontest users, **45** roster users, **41** with Scopus IDs, **zero** non-roster users with Scopus IDs. No current eligible document has a pre-employment eligible author. The apparent equality today does not make these different code rules equivalent.
- `services/scopus_author_role_service.go:64–74` backfill eligibility is broader: any registered Scopus-ID match, without deletion/test/affiliation/employment gates. Hence all 949 can be checked while only 743 qualify for the dashboard.
- Parsing (`:184–230`) accepts `year_start_be`, `year_end_be`, `aggregation_types`, `quality_buckets`, `open_access_mode`, `citation_min/max`, `search_title/doi/eid/scopus_id/journal/author/affiliation/keyword`, and `scope`. BE >=2400 subtracts 543; smaller positive input is treated as CE; invalid year/int values are ignored; reversed years are swapped. Aggregation filters use IN; title/journal/keyword/name/affiliation searches use LIKE; DOI/EID/Scopus ID use equality. Affiliation search currently follows legacy links. OA is either OA column=1; non-OA means both zero/null. Citation bounds are nonnegative.
- Year expression (`:289–291`) is `COALESCE(YEAR(sd.cover_date), CAST(RIGHT(sd.cover_display_date,4) AS UNSIGNED))`; live audit found no missing/zero years. Calendar year is displayed as CE+543. Existing fiscal history moves October–December into the next BE year; filters themselves remain on publication calendar year. New cards should explicitly use **calendar publication year**, matching Faculty Quartile History, unless a later assignment requests fiscal mode. Frontend defaults to the latest three available years (`AdminScopusResearchDashboard.js:374–387`).
- Source quality uses publication-year complete CiteScore metrics, otherwise latest **earlier complete** year, `doc_type='all'`. T1 percentile 90–100 is separate from Q1; Q1–Q4/N/A exclude T1. **Any quality selection excludes Conference Proceeding** in the existing filter helper. Missing quartile becomes N/A. `TCI` is allowed but does not match core metric buckets; TCI-only yields zero core works. Reuse these rules rather than introduce benchmark quartile/category rules.
- Existing summary additionally appends **ThaiJO** when no quality selection or when TCI is selected. Therefore its mixed `total_documents`/history totals cannot be used as the denominator for the three new Scopus-only cards. Use the filtered, distinct **core rows before the ThaiJO merge**. Existing ThaiJO drilldown has its own route branch. Preserve old overview behavior.
- `services/scopus_benchmark_insights.go:199–217`: benchmark verified faculty derives from benchmark scope/year + EID-linked core author and specific KKU AFIDs, with an employment-date gate and no equivalent test-user gate. International detection (`:233–259`) joins benchmark **first affiliation** and calls any recorded country “known,” which can misclassify incomplete Thailand-only metadata as domestic.
- Benchmark summary instead uses national Thailand scope and scope publication years (`scopus_benchmark_summary.go:269–303`), roster role IDs, broader KKU AFID map and CoC map (`:19–20`), all document/author AFIDs, classification/confidence filters, and per-person/overlapping role counts. Its AFID normalization is useful, but its cohort, universe, role counting, and revision cannot be substituted for these dashboard cards.

**Cohort decision for the next phase:** preserve the exact existing dashboard document and eligible-author gate for all three cards, label/version it, and use all affiliations for country detection. Switching faculty eligibility to any author AFID matching the current accepted KKU names would expand the all-year cohort from **743 to 747**: IDs **165, 230, 620, 901**, none in 2024–2026. If the manager chooses this correction later, change the shared dashboard cohort and all affected denominators/drilldowns together, with explicit regression expectations; do not expand only the new cards. Applying the roster or employment gate is likewise a separate cohort decision.

## Recommended schema and normalization (Phase 2 proposal, not applied)

| Core addition | Keys/content | Purpose |
|---|---|---|
| `scopus_document_affiliations` | PK `(document_id, afid)`; reverse index `(afid, document_id)`; nullable catalogue affiliation ID; provenance/evidence bits | Union of top-level and every author AFID; retain unmapped AFIDs |
| `scopus_author_affiliations` | PK `(document_id, author_id, afid)`; reverse `(afid, author_id, document_id)`; provenance | Preserve every author affiliation; keep existing first-affiliation link for compatibility |
| `scopus_document_countries` | PK `(document_id, country_key)`; reverse `(country_key, document_id)`; canonical display name/evidence source | Unique country membership including Thailand; derive partner counts without repeated author joins |
| Document metadata | `international_collaboration` nullable boolean (1=yes, 0=no, NULL=unknown); `affiliations_complete`; `countries_complete`; status/reason codes; `affiliations_checked_at`; normalization version; payload hash; country-catalogue revision | Distinguish presence, completeness, checked/stale state, and reproducible derivation |

Capture expected/provided author counts, missing author-AFID count, unresolved AFID/country count, shape/truncation/conflict reasons, and per-author completeness where useful. Use foreign evidence for `yes` even when incomplete; require nonempty, operationally complete resolved Thailand-only coverage for `no`; all remaining states are unknown. Empty list, absent/null field, missing author count, invalid/mismatched roster, unresolved catalogue, country placeholders/conflicts, or truncation cannot prove domestic. Source country aliases should be deterministic/versioned; store raw spelling and evidence either on affiliation relations or in bounded metadata. Existing raw JSON remains the source audit record.

**Ingest/backfill synchronization:**

1. Shared normalization service consumes the exact saved Search entry and catalogue, without external calls. Online ingest and bounded ID-cursor backfill call the same routine. Parse object/array/scalar `$` forms using the existing parser; distinguish absent/null/empty/malformed. Never use just `Affiliations.First()` for country evidence.
2. Within the existing document transaction, update catalogue rows, all document/author relations, unique country memberships and completeness/tri-state metadata atomically. For a changed, verified complete payload reconcile removed AFIDs/countries. With partial evidence, never declare complete or retain an old domestic assertion; retain stronger older evidence only with explicit provenance/staleness policy, not an unmarked union. A demonstrably newer complete payload may remove obsolete foreign evidence.
3. Hash/version/check metadata makes repeat runs idempotent; backfill scans only core documents and reads saved raw JSON. Dry-run reports deltas/reasons before writes are assigned. Preserve role flags unless author roster/order changes under existing role service rules; do not perform XML harvesting during affiliation backfill.
4. Catalogue country/name corrections affect **every linked document**, including documents not currently being ingested. Use the reverse AFID index to recompute/queue all affected countries/completeness; persist a dirty/revision state atomically. API must treat stale country classification as unknown or recompute before returning it. Never expose stale `no` after a country mapping changes. Name/user-ID/test/deletion changes can alter cohort membership and must invalidate relevant results too.
5. Keep legacy first-affiliation fallback as **positive evidence only**, marked incomplete. Do not overwrite reliable document-payload countries with an empty catalogue value. Conflicting nonempty countries need reason/provenance and a conservative documented resolution; no conflict occurred in this snapshot.

The benchmark helper is not safe to copy unmodified: `replaceBenchmarkAffiliationMetadata` sets document `affiliations_complete=true` whenever `affiliation` is present/non-null, regardless of empty/malformed list, truncated roster, missing country, or missing author AFIDs. Author completeness uses non-nil AFIDs (an empty array can qualify), and flags are only set true, not consistently reset on a later partial payload. Reconciliation/merged core/benchmark evidence can retain stale relations. Migration 049 provides useful relation keys and provenance, **not certified completeness**.

## API, cache, UI, and phase plan

**Phase 3 — API:** add `GET /api/v1/admin/scopus/dashboard/faculty-insights` and `GET /api/v1/admin/scopus/dashboard/faculty-insights/drilldown`, guarded by the same `ui.page.admin.research_dashboard.view` permission as dashboard routes (`routes/routes.go:631–633`). Add methods next to existing wrappers in frontend `app/lib/admin_api.js:720–749`. Reuse `parseScopusDashboardFilters`, metric-year selection and shared distinct core-document/eligible-author query; force faculty semantics for these cards. All aggregates in one consistent snapshot; do not read benchmark or ThaiJO tables.

Summary response should include applied filters, calendar `years_be`, cohort/version and normalization revision; `international.yearly` + selected-range totals (`total_scopus`, `yes`, `no`, `unknown`, `international_pct`); sorted `partners` (`country_key`, `label`, `unique_documents`); `roles.yearly` + selected-range counts (`first`, `corresponding`, `coauthor`, `unknown`, `total`); and `country_roles` for all **yes/no/unknown** rows with the same four role columns. Include completeness/stale coverage and generated time. Count each document once per role and once per country. Country percentages use all filtered Scopus works; each role donut uses its own population, includes unknown role, and supplies its denominator. Return `null` percentage for a zero denominator and zero counts; do not label an empty set 0% certainty. All-time unknown publication year, if present later, must reconcile in an explicit undated bucket rather than silently disappear from totals.

Drilldown applies the same filter snapshot plus optional `year_be`, `international_status=yes|no|unknown`, `role=first|corresponding|coauthor|unknown`, and `country_key`. Filters combine by AND; no year means the selected range. Page=1 default, page_size=25 default, max=200, stable publication-year/ID ordering. Return **server-paged** documents, total/pages, EID/Scopus link, BE year, status and completeness reasons, stored/display countries including Thailand, exclusive faculty role and bounded supporting faculty role evidence. Reject invalid insight dimensions with 400. Summary revision should be returned/accepted in drilldown so a changed snapshot can prompt refresh instead of silently presenting mismatched counts.

Live indexes: core unique EID; author unique Scopus author ID; affiliations unique AFID; links unique `(document_id,author_id)`, `(document_id,author_seq)`, author/affiliation indexes; documents cover_date/source_id/DOI/role-status indexes; metrics unique `(source_id,metric_year,doc_type)` and composite lookup including status. No users Scopus-ID index. Recorded EXPLAIN scans about 59 users, then indexed author/link/document/affiliation lookups and temporary deduplication; function-wrapped year prevents a simple cover_date range scan. Existing scale (949) is small. Add relation reverse indexes above; measure before adding normalized user-ID or generated publication-year indexes. Do not add speculative role flag indexes instead of measuring document-first aggregates. Save EXPLAIN for actual filtered/partner/drilldown queries after implementation.

Current dashboard cache is process-local, filter options **6 hours**, summary **10 minutes**, with `refresh=1` bypass and keys `filter_options_v3` / `summary_v11:<filters>`. No ingest/role/catalogue mutation invalidation was found. New summary/drilldown must not disagree due to old cached counts. Prefer a database-backed core insight/cohort revision in cache keys (plus canonical filters and normalization version) so all app instances observe updates; dirty-country changes also advance it. Successful ingest, country backfill/catalogue edit, XML role update/prune, user cohort change, and source-metric change must invalidate the relevant cache/revision **after commit**. Include checked/provenance changes when shown in drilldown. If a safe shared revision is not ready, ship the insight endpoint uncached initially; TTL alone does not satisfy immediate invalidation. Benchmark export SHA revision illustrates dependency coverage but is not a dashboard invalidation mechanism.

**Phase 4 — UI:** add three collapsible `SimpleCard`s immediately after Faculty Quartile History (`AdminScopusResearchDashboard.js:3764–3837`), before `AdminScopusFacultyHIndex`, within faculty overview. Fetch using **appliedFilters**, via `filterToQueryParams` (`:345–371`), not draft controls. Keep loading/error/empty/stale states explicit, discard stale responses on filter changes, reset drilldown page/selection. Reuse dynamic ApexChart and existing visual/table styles.

1. International collaboration: yearly table + totals with all Scopus/yes/no/unknown/percentage; horizontal partner-country bar chart excluding Thailand. Table cells and bars open insight drilldown; explain unique-country counts may exceed international papers and dual affiliation operational definition.
2. Faculty roles: yearly table and selected-range donut with First/Corresponding/Co-author/Unknown. BE range heading, fixed colors/legend, counts + percentages, explicit precedence explanation; no person-level summation.
3. International vs domestic faculty roles: two side-by-side donuts (stacked on narrow screens) with identical role order/colors and their respective denominators; cross-tab includes unknown-country row and unknown-role column, totals and clickable counts. Show unknown-country coverage beside the comparison. Do not place unknown-country works into domestic.

Reuse the drilldown presentation where practical, but implement server paging: the existing overview loads only the first **200** and then pages in the browser (`:121`, `:840–852`), which can hide records for larger new populations. Preserve all filters and selected dimension across pagination. Do not add these cards to existing PDF output without explicit export scope/verification; current overview export targets a different section.

**Phase 5 — acceptance and validation:**

- Parser/normalizer fixtures: multiple AFIDs, scalar/object/array `$` forms, duplicate affiliations/countries, dual affiliation, missing/null/empty/malformed lists, absent/bad/count-mismatched/truncated authors, unknown AFID/country, alias/whitespace/case and conflicting mappings. Thailand-only incomplete => unknown; any verified foreign => yes; complete Thailand-only => no. Foreign evidence survives partial metadata with declared provenance.
- Role fixtures: same faculty first+corr+co => one First; known First + unknown => First; Corresponding + another unresolved eligible author => Unknown; fully known no First + Corresponding => Corresponding; all confirmed neither => Co-author; no eligible faculty, or stale/pending roles without usable positive First evidence => Unknown. Ineligible external first/corresponding flags do not change faculty role.
- Ingest/backfill integration: repeat is idempotent; complete newer payload removes stale AFIDs/countries; partial payload resets completeness/domestic certainty; catalogue country add/change/removal recomputes every affected doc; roster change invalidates roles; transaction failure leaves no partially synchronized countries. No backfill harvests external APIs.
- Live **development** parity baseline (if data/revisions unchanged): 743 all-year faculty; 226 selected-year total; years 68/81/77; international 37/35/50; aggregate 122/104/0; roles 20/82/124/0; cross-tab above; all-year unknown countries 302/303/503; 294 partner memberships vs 122 works. IDs 912/721/953 must appear in foreign drilldowns. Document 3 appears once in First. Four expanded-cohort IDs must stay excluded under the preserved gate.
- Every API filter separately and representative combinations (including TCI-only => zero core, conferences + quality behavior, metric fallback, reversed BE years, OA/citation/text/affiliation searches) must give identical summary/drilldown populations. Zero/undated years, page boundaries >200, duplicate user IDs and author joins, invalid dimensions, permission errors, and concurrent data updates/revision mismatch require coverage. Assert country and role partitions each sum to all filtered Scopus documents; cross-tab row/column totals reconcile; percentages derive from declared denominators.
- Cache tests verify an ingest/role/catalogue/user/metric mutation changes revision or leaves results uncached across instances; rejected/rolled-back mutations do not publish false completed revisions.
- Frontend lint/build and API/unit/integration checks appropriate to the assigned phases; browser verify BE 2567–2569 counts, paired colors/denominators, unknown/empty/error/loading states, mobile layout, clicked countries/roles/year totals, applied vs draft filter behavior, and server pagination. Use test fixtures for unknown roles/countries absent from the selected-year live baseline. No production assertion from these development numbers.

## Phase 1 validation and handoff

Read actual backend models/controllers/ingest/role/benchmark/migration code and frontend filter/chart/drilldown integration. Verified live schema and indexes and READ ONLY session state, audited the complete bounded core dataset and exact current dashboard subset, and recorded reproducible SQL plus publication evidence. Helper syntax and result partition checks are validated locally. Product tests/builds are not required for this documentation-only phase; no product files were edited.

Changed files are this report, `docs/audits/faculty_insights_phase1.py`, and `docs/audits/faculty_insights_phase1_results.json`. The frontend remains unchanged. No credentials were printed and `cmd/test-db` was not run. **READY FOR MANAGER REVIEW. Await Phase 2 assignment; manager owns acceptance and commits on the existing branch.**
