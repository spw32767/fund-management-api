-- Scopus dashboard validation queries
-- These queries mirror the controller logic in:
-- controllers/admin_scopus_dashboard_controller.go

-- =============================
-- 0) Parameters (edit as needed)
-- =============================
SET @scope := 'individual';
SET @year_start_ce := NULL;
SET @year_end_ce := NULL;
SET @aggregation_types_csv := '';
SET @quality_buckets_csv := '';
SET @open_access_mode := 'all';
SET @citation_min := NULL;
SET @citation_max := NULL;
SET @search_title := '';
SET @search_doi := '';
SET @search_eid := '';
SET @search_scopus_id := '';
SET @search_journal := '';
SET @search_author := '';
SET @search_affiliation := '';
SET @search_keyword := '';

-- quality examples:
-- SET @quality_buckets_csv := 'T1,Q1,Q2,Q3,Q4,N/A';
-- SET @quality_buckets_csv := 'T1';
-- SET @quality_buckets_csv := 'Q1,Q2';

-- aggregation examples:
-- SET @aggregation_types_csv := 'Journal,Conference Proceeding';


-- =====================================================
-- 1) Person Summary validation (exclusive T1 from Q1)
-- =====================================================
WITH doc_base AS (
  SELECT
    u.user_id,
    TRIM(CONCAT(COALESCE(u.user_fname, ''), ' ', COALESCE(u.user_lname, ''))) AS user_name,
    COALESCE(NULLIF(TRIM(u.email), ''), '-') AS user_email,
    COALESCE(NULLIF(TRIM(u.scopus_id), ''), '-') AS user_scopus_id,
    sd.id AS document_id,
    COALESCE(YEAR(sd.cover_date), CAST(RIGHT(sd.cover_display_date, 4) AS UNSIGNED)) AS publication_year_ce,
    COALESCE(sd.citedby_count, 0) AS cited_by_count,
    COALESCE(NULLIF(UPPER(TRIM(metrics.cite_score_quartile)), ''), 'N/A') AS quartile,
    metrics.cite_score_percentile,
    COALESCE(NULLIF(TRIM(sd.aggregation_type), ''), 'N/A') AS aggregation_type
  FROM users u
  JOIN scopus_authors sa ON TRIM(u.scopus_id) = sa.scopus_author_id
  JOIN scopus_document_authors sda ON sda.author_id = sa.id
  JOIN scopus_documents sd ON sd.id = sda.document_id
  LEFT JOIN scopus_source_metrics metrics
    ON metrics.source_id = sd.source_id
   AND metrics.doc_type = 'all'
   AND metrics.metric_year = COALESCE(
      (
        SELECT ssm_complete.metric_year
        FROM scopus_source_metrics ssm_complete
        WHERE ssm_complete.source_id = sd.source_id
          AND ssm_complete.doc_type = 'all'
          AND ssm_complete.metric_year = COALESCE(YEAR(sd.cover_date), CAST(RIGHT(sd.cover_display_date, 4) AS UNSIGNED))
          AND LOWER(ssm_complete.cite_score_status) = 'complete'
        LIMIT 1
      ),
      CASE
        WHEN EXISTS (
          SELECT 1
          FROM scopus_source_metrics ssm_ip
          WHERE ssm_ip.source_id = sd.source_id
            AND ssm_ip.doc_type = 'all'
            AND ssm_ip.metric_year = COALESCE(YEAR(sd.cover_date), CAST(RIGHT(sd.cover_display_date, 4) AS UNSIGNED))
            AND LOWER(ssm_ip.cite_score_status) = 'in-progress'
        )
        THEN (
          SELECT MAX(ssm_prev.metric_year)
          FROM scopus_source_metrics ssm_prev
          WHERE ssm_prev.source_id = sd.source_id
            AND ssm_prev.doc_type = 'all'
            AND ssm_prev.metric_year < COALESCE(YEAR(sd.cover_date), CAST(RIGHT(sd.cover_display_date, 4) AS UNSIGNED))
            AND LOWER(ssm_prev.cite_score_status) = 'complete'
        )
        ELSE NULL
      END
   )
  WHERE u.delete_at IS NULL
    AND u.scopus_id IS NOT NULL
    AND TRIM(u.scopus_id) <> ''
    AND (
      @scope <> 'individual'
      OR EXISTS (
        SELECT 1
        FROM scopus_document_authors sda2
        JOIN scopus_authors sa2 ON sa2.id = sda2.author_id
        JOIN users u2 ON TRIM(u2.scopus_id) = sa2.scopus_author_id
        WHERE sda2.document_id = sd.id
          AND u2.scopus_id IS NOT NULL
          AND TRIM(u2.scopus_id) <> ''
      )
    )
    AND (@year_start_ce IS NULL OR COALESCE(YEAR(sd.cover_date), CAST(RIGHT(sd.cover_display_date, 4) AS UNSIGNED)) >= @year_start_ce)
    AND (@year_end_ce IS NULL OR COALESCE(YEAR(sd.cover_date), CAST(RIGHT(sd.cover_display_date, 4) AS UNSIGNED)) <= @year_end_ce)
    AND (@aggregation_types_csv = '' OR FIND_IN_SET(sd.aggregation_type, @aggregation_types_csv) > 0)
    AND (
      @open_access_mode = 'all'
      OR (@open_access_mode = 'oa' AND (COALESCE(sd.openaccess_flag, 0) = 1 OR COALESCE(sd.openaccess, 0) = 1))
      OR (@open_access_mode = 'non_oa' AND (COALESCE(sd.openaccess_flag, 0) = 0 AND COALESCE(sd.openaccess, 0) = 0))
    )
    AND (@citation_min IS NULL OR COALESCE(sd.citedby_count, 0) >= @citation_min)
    AND (@citation_max IS NULL OR COALESCE(sd.citedby_count, 0) <= @citation_max)
    AND (@search_title = '' OR sd.title LIKE CONCAT('%', @search_title, '%'))
    AND (@search_doi = '' OR sd.doi = @search_doi)
    AND (@search_eid = '' OR sd.eid = @search_eid)
    AND (@search_scopus_id = '' OR sd.scopus_id = @search_scopus_id)
    AND (@search_journal = '' OR sd.publication_name LIKE CONCAT('%', @search_journal, '%'))
    AND (@search_keyword = '' OR sd.authkeywords LIKE CONCAT('%', @search_keyword, '%'))
    AND (
      @search_author = ''
      OR EXISTS (
        SELECT 1
        FROM scopus_document_authors sda3
        JOIN scopus_authors sa3 ON sa3.id = sda3.author_id
        WHERE sda3.document_id = sd.id
          AND (
            sa3.full_name LIKE CONCAT('%', @search_author, '%')
            OR sa3.given_name LIKE CONCAT('%', @search_author, '%')
            OR sa3.surname LIKE CONCAT('%', @search_author, '%')
          )
      )
    )
    AND (
      @search_affiliation = ''
      OR EXISTS (
        SELECT 1
        FROM scopus_document_authors sda4
        JOIN scopus_affiliations aff ON aff.id = sda4.affiliation_id
        WHERE sda4.document_id = sd.id
          AND (
            aff.name LIKE CONCAT('%', @search_affiliation, '%')
            OR aff.city LIKE CONCAT('%', @search_affiliation, '%')
            OR aff.country LIKE CONCAT('%', @search_affiliation, '%')
            OR aff.afid LIKE CONCAT('%', @search_affiliation, '%')
          )
      )
    )
    AND (
      @quality_buckets_csv = ''
      OR (
        (FIND_IN_SET('T1', @quality_buckets_csv) > 0 AND metrics.cite_score_percentile BETWEEN 90 AND 100)
        OR (
          (FIND_IN_SET('Q1', @quality_buckets_csv) > 0 AND COALESCE(NULLIF(UPPER(TRIM(metrics.cite_score_quartile)), ''), 'N/A') = 'Q1')
          OR (FIND_IN_SET('Q2', @quality_buckets_csv) > 0 AND COALESCE(NULLIF(UPPER(TRIM(metrics.cite_score_quartile)), ''), 'N/A') = 'Q2')
          OR (FIND_IN_SET('Q3', @quality_buckets_csv) > 0 AND COALESCE(NULLIF(UPPER(TRIM(metrics.cite_score_quartile)), ''), 'N/A') = 'Q3')
          OR (FIND_IN_SET('Q4', @quality_buckets_csv) > 0 AND COALESCE(NULLIF(UPPER(TRIM(metrics.cite_score_quartile)), ''), 'N/A') = 'Q4')
          OR (FIND_IN_SET('N/A', @quality_buckets_csv) > 0 AND COALESCE(NULLIF(UPPER(TRIM(metrics.cite_score_quartile)), ''), 'N/A') = 'N/A')
        )
      )
    )
),
dedup AS (
  SELECT DISTINCT
    user_id,
    user_name,
    user_email,
    user_scopus_id,
    document_id,
    publication_year_ce,
    cited_by_count,
    quartile,
    cite_score_percentile,
    aggregation_type
  FROM doc_base
)
SELECT
  d.user_id,
  d.user_name,
  d.user_email,
  d.user_scopus_id,
  COUNT(*) AS unique_documents,
  SUM(d.cited_by_count) AS cited_by_total,
  ROUND(SUM(d.cited_by_count) / NULLIF(COUNT(*), 0), 2) AS avg_cited_by,
  SUM(CASE WHEN d.cite_score_percentile BETWEEN 90 AND 100 THEN 1 ELSE 0 END) AS t1_count,
  SUM(CASE WHEN d.cite_score_percentile NOT BETWEEN 90 AND 100 AND d.quartile = 'Q1' THEN 1 ELSE 0 END) AS q1_count,
  SUM(CASE WHEN d.cite_score_percentile NOT BETWEEN 90 AND 100 AND d.quartile = 'Q2' THEN 1 ELSE 0 END) AS q2_count,
  SUM(CASE WHEN d.cite_score_percentile NOT BETWEEN 90 AND 100 AND d.quartile = 'Q3' THEN 1 ELSE 0 END) AS q3_count,
  SUM(CASE WHEN d.cite_score_percentile NOT BETWEEN 90 AND 100 AND d.quartile = 'Q4' THEN 1 ELSE 0 END) AS q4_count,
  SUM(CASE WHEN d.cite_score_percentile NOT BETWEEN 90 AND 100 AND d.quartile NOT IN ('Q1', 'Q2', 'Q3', 'Q4') THEN 1 ELSE 0 END) AS quartile_na,
  SUM(CASE WHEN d.aggregation_type = 'Journal' THEN 1 ELSE 0 END) AS journal_count,
  SUM(CASE WHEN d.aggregation_type = 'Conference Proceeding' THEN 1 ELSE 0 END) AS conference_count,
  CASE WHEN MIN(d.publication_year_ce) IS NULL THEN 0 ELSE MIN(d.publication_year_ce) + 543 END AS first_year,
  CASE WHEN MAX(d.publication_year_ce) IS NULL THEN 0 ELSE MAX(d.publication_year_ce) + 543 END AS latest_year,
  COUNT(DISTINCT d.publication_year_ce) AS active_years
FROM dedup d
GROUP BY d.user_id, d.user_name, d.user_email, d.user_scopus_id
ORDER BY unique_documents DESC, cited_by_total DESC, user_name ASC;


-- ====================================================================
-- 2) Person Year Matrix validation (same filter scope as Person Summary)
-- ====================================================================
WITH doc_base AS (
  SELECT
    u.user_id,
    TRIM(CONCAT(COALESCE(u.user_fname, ''), ' ', COALESCE(u.user_lname, ''))) AS user_name,
    COALESCE(NULLIF(TRIM(u.email), ''), '-') AS user_email,
    COALESCE(NULLIF(TRIM(u.scopus_id), ''), '-') AS user_scopus_id,
    sd.id AS document_id,
    COALESCE(YEAR(sd.cover_date), CAST(RIGHT(sd.cover_display_date, 4) AS UNSIGNED)) AS publication_year_ce,
    COALESCE(NULLIF(UPPER(TRIM(metrics.cite_score_quartile)), ''), 'N/A') AS quartile,
    metrics.cite_score_percentile
  FROM users u
  JOIN scopus_authors sa ON TRIM(u.scopus_id) = sa.scopus_author_id
  JOIN scopus_document_authors sda ON sda.author_id = sa.id
  JOIN scopus_documents sd ON sd.id = sda.document_id
  LEFT JOIN scopus_source_metrics metrics
    ON metrics.source_id = sd.source_id
   AND metrics.doc_type = 'all'
   AND metrics.metric_year = COALESCE(
      (
        SELECT ssm_complete.metric_year
        FROM scopus_source_metrics ssm_complete
        WHERE ssm_complete.source_id = sd.source_id
          AND ssm_complete.doc_type = 'all'
          AND ssm_complete.metric_year = COALESCE(YEAR(sd.cover_date), CAST(RIGHT(sd.cover_display_date, 4) AS UNSIGNED))
          AND LOWER(ssm_complete.cite_score_status) = 'complete'
        LIMIT 1
      ),
      CASE
        WHEN EXISTS (
          SELECT 1
          FROM scopus_source_metrics ssm_ip
          WHERE ssm_ip.source_id = sd.source_id
            AND ssm_ip.doc_type = 'all'
            AND ssm_ip.metric_year = COALESCE(YEAR(sd.cover_date), CAST(RIGHT(sd.cover_display_date, 4) AS UNSIGNED))
            AND LOWER(ssm_ip.cite_score_status) = 'in-progress'
        )
        THEN (
          SELECT MAX(ssm_prev.metric_year)
          FROM scopus_source_metrics ssm_prev
          WHERE ssm_prev.source_id = sd.source_id
            AND ssm_prev.doc_type = 'all'
            AND ssm_prev.metric_year < COALESCE(YEAR(sd.cover_date), CAST(RIGHT(sd.cover_display_date, 4) AS UNSIGNED))
            AND LOWER(ssm_prev.cite_score_status) = 'complete'
        )
        ELSE NULL
      END
   )
  WHERE u.delete_at IS NULL
    AND u.scopus_id IS NOT NULL
    AND TRIM(u.scopus_id) <> ''
    AND (
      @scope <> 'individual'
      OR EXISTS (
        SELECT 1
        FROM scopus_document_authors sda2
        JOIN scopus_authors sa2 ON sa2.id = sda2.author_id
        JOIN users u2 ON TRIM(u2.scopus_id) = sa2.scopus_author_id
        WHERE sda2.document_id = sd.id
          AND u2.scopus_id IS NOT NULL
          AND TRIM(u2.scopus_id) <> ''
      )
    )
    AND (@year_start_ce IS NULL OR COALESCE(YEAR(sd.cover_date), CAST(RIGHT(sd.cover_display_date, 4) AS UNSIGNED)) >= @year_start_ce)
    AND (@year_end_ce IS NULL OR COALESCE(YEAR(sd.cover_date), CAST(RIGHT(sd.cover_display_date, 4) AS UNSIGNED)) <= @year_end_ce)
    AND (@aggregation_types_csv = '' OR FIND_IN_SET(sd.aggregation_type, @aggregation_types_csv) > 0)
    AND (
      @open_access_mode = 'all'
      OR (@open_access_mode = 'oa' AND (COALESCE(sd.openaccess_flag, 0) = 1 OR COALESCE(sd.openaccess, 0) = 1))
      OR (@open_access_mode = 'non_oa' AND (COALESCE(sd.openaccess_flag, 0) = 0 AND COALESCE(sd.openaccess, 0) = 0))
    )
    AND (@citation_min IS NULL OR COALESCE(sd.citedby_count, 0) >= @citation_min)
    AND (@citation_max IS NULL OR COALESCE(sd.citedby_count, 0) <= @citation_max)
    AND (@search_title = '' OR sd.title LIKE CONCAT('%', @search_title, '%'))
    AND (@search_doi = '' OR sd.doi = @search_doi)
    AND (@search_eid = '' OR sd.eid = @search_eid)
    AND (@search_scopus_id = '' OR sd.scopus_id = @search_scopus_id)
    AND (@search_journal = '' OR sd.publication_name LIKE CONCAT('%', @search_journal, '%'))
    AND (@search_keyword = '' OR sd.authkeywords LIKE CONCAT('%', @search_keyword, '%'))
    AND (
      @search_author = ''
      OR EXISTS (
        SELECT 1
        FROM scopus_document_authors sda3
        JOIN scopus_authors sa3 ON sa3.id = sda3.author_id
        WHERE sda3.document_id = sd.id
          AND (
            sa3.full_name LIKE CONCAT('%', @search_author, '%')
            OR sa3.given_name LIKE CONCAT('%', @search_author, '%')
            OR sa3.surname LIKE CONCAT('%', @search_author, '%')
          )
      )
    )
    AND (
      @search_affiliation = ''
      OR EXISTS (
        SELECT 1
        FROM scopus_document_authors sda4
        JOIN scopus_affiliations aff ON aff.id = sda4.affiliation_id
        WHERE sda4.document_id = sd.id
          AND (
            aff.name LIKE CONCAT('%', @search_affiliation, '%')
            OR aff.city LIKE CONCAT('%', @search_affiliation, '%')
            OR aff.country LIKE CONCAT('%', @search_affiliation, '%')
            OR aff.afid LIKE CONCAT('%', @search_affiliation, '%')
          )
      )
    )
    AND (
      @quality_buckets_csv = ''
      OR (
        (FIND_IN_SET('T1', @quality_buckets_csv) > 0 AND metrics.cite_score_percentile BETWEEN 90 AND 100)
        OR (
          (FIND_IN_SET('Q1', @quality_buckets_csv) > 0 AND COALESCE(NULLIF(UPPER(TRIM(metrics.cite_score_quartile)), ''), 'N/A') = 'Q1')
          OR (FIND_IN_SET('Q2', @quality_buckets_csv) > 0 AND COALESCE(NULLIF(UPPER(TRIM(metrics.cite_score_quartile)), ''), 'N/A') = 'Q2')
          OR (FIND_IN_SET('Q3', @quality_buckets_csv) > 0 AND COALESCE(NULLIF(UPPER(TRIM(metrics.cite_score_quartile)), ''), 'N/A') = 'Q3')
          OR (FIND_IN_SET('Q4', @quality_buckets_csv) > 0 AND COALESCE(NULLIF(UPPER(TRIM(metrics.cite_score_quartile)), ''), 'N/A') = 'Q4')
          OR (FIND_IN_SET('N/A', @quality_buckets_csv) > 0 AND COALESCE(NULLIF(UPPER(TRIM(metrics.cite_score_quartile)), ''), 'N/A') = 'N/A')
        )
      )
    )
),
dedup AS (
  SELECT DISTINCT
    user_id,
    user_name,
    user_email,
    user_scopus_id,
    document_id,
    publication_year_ce
  FROM doc_base
)
SELECT
  d.user_id,
  d.user_name,
  d.user_email,
  d.user_scopus_id,
  d.publication_year_ce + 543 AS publication_year_be,
  COUNT(*) AS unique_documents
FROM dedup d
WHERE d.publication_year_ce IS NOT NULL
GROUP BY
  d.user_id,
  d.user_name,
  d.user_email,
  d.user_scopus_id,
  d.publication_year_ce
ORDER BY publication_year_be ASC, user_name ASC;


-- ======================================================================
-- 3) Internal collaboration pairs validation (intentionally unfiltered)
-- ======================================================================
SELECT
  d1.user_id AS user_a_id,
  TRIM(CONCAT(COALESCE(ua.user_fname, ''), ' ', COALESCE(ua.user_lname, ''))) AS user_a,
  d2.user_id AS user_b_id,
  TRIM(CONCAT(COALESCE(ub.user_fname, ''), ' ', COALESCE(ub.user_lname, ''))) AS user_b,
  COUNT(DISTINCT d1.document_id) AS shared_documents
FROM (
  SELECT DISTINCT u.user_id, sda.document_id
  FROM users u
  JOIN scopus_authors sa ON TRIM(u.scopus_id) = sa.scopus_author_id
  JOIN scopus_document_authors sda ON sda.author_id = sa.id
  WHERE u.delete_at IS NULL
    AND u.scopus_id IS NOT NULL
    AND TRIM(u.scopus_id) <> ''
) d1
JOIN (
  SELECT DISTINCT u.user_id, sda.document_id
  FROM users u
  JOIN scopus_authors sa ON TRIM(u.scopus_id) = sa.scopus_author_id
  JOIN scopus_document_authors sda ON sda.author_id = sa.id
  WHERE u.delete_at IS NULL
    AND u.scopus_id IS NOT NULL
    AND TRIM(u.scopus_id) <> ''
) d2 ON d1.document_id = d2.document_id AND d1.user_id < d2.user_id
JOIN users ua ON ua.user_id = d1.user_id
JOIN users ub ON ub.user_id = d2.user_id
GROUP BY d1.user_id, user_a, d2.user_id, user_b
HAVING COUNT(DISTINCT d1.document_id) >= 1
ORDER BY shared_documents DESC, user_a ASC, user_b ASC;
