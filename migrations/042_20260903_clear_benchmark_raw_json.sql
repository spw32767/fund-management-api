-- BE-2 retention cleanup. Run only after the affiliation coverage checks from
-- docs/scopus_benchmark_affiliation_verification.sql pass.
-- Keep the nullable column for rollback compatibility; only clear its payload.

UPDATE scopus_benchmark_documents
SET raw_json = NULL
WHERE raw_json IS NOT NULL;

OPTIMIZE TABLE scopus_benchmark_documents;
