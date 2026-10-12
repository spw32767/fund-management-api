# Faculty drilldown server sorting

Branch: `codex/faculty-drilldown-sorting`, based on `2644d67a6531a66442986a9fd16b4450c98fae3d`. Changes are uncommitted for manager review.

The existing read-only faculty drilldown accepts `sort_field=title|year|collaboration|faculty_role` together with `sort_direction=asc|desc`. Omitting both defaults to **year descending (newest first)**, with unknown years last and document ID ascending for ties. Invalid, empty or incomplete pairs return 400 using the existing dimension error code. Response `sort` is `year_desc` by default and echoes `sort_field=year`/`sort_direction=desc`.

The full snapshot is still read and hashed in ID order. After revision validation, dimensions and search, a separate matching slice is sorted in Go before page slicing. Request values are strictly allowlisted and never interpolated into SQL. Counts, selection predicates, permissions, summary filters, query count and revision hashing remain unchanged.

Text keys are trimmed, Unicode-lowercased and compared in Unicode code-point lexical order, not locale dictionary or natural numeric order. Year is numeric. Collaboration compares displayed Thai status (`ภายในประเทศ` before `ร่วมกับต่างประเทศ` ascending), then the existing displayed country list, translating `Thailand` to `ประเทศไทย`. Unknown status, stale evidence or an empty country list puts the whole composite last. Roles compare displayed labels (`Co-author`, `Corresponding author`, `First author` ascending). Missing titles/years and unknown roles remain last. Equal keys and missing values always tie by document ID ascending, including descending sorts.

New unit/integration tests cover field/direction validation, Unicode/case ties, null/blank values, both directions, numeric years, country composites, all fields, >200 selected records, ordering before pagination, search with dimensions, old defaults, no duplicate/lost records across pages, revision mismatch and unchanged summary revision. One pre-existing integration permission test was updated for the merged effective-grant middleware, covering teacher/admin with and without the dashboard grant. No product permission behavior changed.

Validation passed:

The newest-first follow-up is based on accepted backend `2daaf96` and frontend `12b3aa6`. Its separate evidence is under `faculty-drilldown-sort-evidence/newest-year-default`; original feature evidence is preserved. Updated tests cover omitted sort parameters, newest-first years, stable ID ties and undated documents last.

[Follow-up full isolated integration suite](faculty-drilldown-sort-evidence/newest-year-default/backend-integration.txt) passed for controllers/services/routes. The frontend follow-up report includes 121 passing unit tests and 8 passing desktop/mobile checks. Logs below belong to the original accepted feature.

- [Backend regression](faculty-drilldown-sort-evidence/backend-regression.txt): CGO disabled, controllers/models/services/routes/insights CLI.
- [Isolated SQLite integration](faculty-drilldown-sort-evidence/backend-integration.txt): controllers/services/routes, `insight_integration`, GOARCH=386/CGO=1/installed `F:\MinGW\bin\gcc.exe`. All database writes in this validation are isolated fixture setup, never application data.

See the [frontend report, state semantics and screenshots](../../frontend_project_fund/docs/SCOPUS_FACULTY_DRILLDOWN_SORTING.md) for the coordinated UI implementation. No application DB writes, migration, deploy, push, commit, or user-service restart was performed.
