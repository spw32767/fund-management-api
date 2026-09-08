-- BE-2 affiliation verification and example analysis queries.
-- Run against the dev database after migration 041 + backfill and before 042.

-- 1) Dashboard isolation: compare these counts with the pre-work baseline.
SELECT COUNT(*) AS scopus_affiliations_count
FROM scopus_affiliations;

SELECT COUNT(*) AS scopus_document_authors_count
FROM scopus_document_authors;

-- 2) Every raw document that contains affiliation metadata must have at least
-- one benchmark author-affiliation link. This must return zero before 042.
SELECT COUNT(*) AS documents_without_affiliation_link
FROM scopus_benchmark_documents AS d
WHERE d.raw_json IS NOT NULL
  AND LENGTH(d.raw_json) > 0
  AND JSON_LENGTH(JSON_EXTRACT(d.raw_json, '$.affiliation')) > 0
  AND NOT EXISTS (
    SELECT 1
    FROM scopus_benchmark_document_authors AS da
    WHERE da.document_id = d.id
      AND da.affiliation_id IS NOT NULL
  );

-- 3) Link coverage summary.
SELECT
  COUNT(*) AS total_author_links,
  SUM(da.affiliation_id IS NOT NULL) AS linked_author_links,
  ROUND(100 * SUM(da.affiliation_id IS NOT NULL) / COUNT(*), 2) AS linked_percent
FROM scopus_benchmark_document_authors AS da;

-- 4) Top countries represented by each author's first Scopus AF-ID.
SELECT
  COALESCE(NULLIF(TRIM(a.country), ''), '(unknown)') AS country,
  COUNT(DISTINCT da.document_id) AS documents
FROM scopus_benchmark_document_authors AS da
JOIN scopus_benchmark_affiliations AS a ON a.id = da.affiliation_id
GROUP BY country
ORDER BY documents DESC;

-- 5) Top institutions represented by each author's first Scopus AF-ID.
SELECT
  a.afid,
  a.name,
  a.country,
  COUNT(DISTINCT da.document_id) AS documents
FROM scopus_benchmark_document_authors AS da
JOIN scopus_benchmark_affiliations AS a ON a.id = da.affiliation_id
GROUP BY a.id, a.afid, a.name, a.country
ORDER BY documents DESC;

-- 6) International collaboration by publication year: a document has at least
-- one KKU/Faculty of Science author and at least one first affiliation outside Thailand.
SELECT
  d.pub_year,
  COUNT(DISTINCT d.id) AS international_collaboration_documents
FROM scopus_benchmark_documents AS d
WHERE EXISTS (
  SELECT 1
  FROM scopus_benchmark_document_authors AS da
  JOIN scopus_benchmark_affiliations AS a ON a.id = da.affiliation_id
  WHERE da.document_id = d.id
    AND a.afid IN ('60017165', '60280609')
)
AND EXISTS (
  SELECT 1
  FROM scopus_benchmark_document_authors AS da
  JOIN scopus_benchmark_affiliations AS a ON a.id = da.affiliation_id
  WHERE da.document_id = d.id
    AND COALESCE(TRIM(a.country), '') <> ''
    AND LOWER(TRIM(a.country)) <> 'thailand'
)
GROUP BY d.pub_year
ORDER BY d.pub_year;
