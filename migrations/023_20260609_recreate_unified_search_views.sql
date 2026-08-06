-- ============================================================
-- Index: ช่วย performance สำหรับ subquery ใน Scopus Metrics
-- ============================================================
CREATE INDEX IF NOT EXISTS idx_metrics_lookup
  ON scopus_source_metrics(source_id, doc_type, metric_year, cite_score_status);

-- ============================================================
-- View : unified_search_contents
-- ============================================================
CREATE OR REPLACE VIEW unified_search_contents AS

SELECT
    CONCAT('scopus_', d.id) COLLATE utf8mb4_general_ci AS id,
    'scopus' COLLATE utf8mb4_general_ci             AS source_name,
    COALESCE(d.title, 'Untitled')                   AS title,
    NULL                                             AS title_en,
    d.abstract                                       AS abstract,
    'faculty' COLLATE utf8mb4_general_ci             AS publication_type,
    COALESCE(
      YEAR(d.cover_date),
      CAST(RIGHT(d.cover_display_date, 4) AS UNSIGNED)
    )                                                AS publication_year,
    d.id                                             AS source_id,
    d.aggregation_type                               AS detail_type,
    d.citedby_count                                  AS cited_by,
    NULL                                             AS track_id,
    CASE WHEN d.aggregation_type = 'Conference Proceeding' THEN NULL ELSE NULLIF(UPPER(TRIM(metrics.cite_score_quartile)), '') END AS journal_quartile,
    metrics.cite_score_percentile                    AS journal_percentile,
    NULL                                             AS journal_tier,
    d.publication_name                               AS journal_name,
    d.scopus_link                                    AS url,
    d.authkeywords                                   AS keywords,
    NULL                                             AS poster_url,
    NULL                                             AS group_code,
    COALESCE(
        d.cover_date,
        MAKEDATE(
            COALESCE(YEAR(d.cover_date), CAST(RIGHT(d.cover_display_date, 4) AS UNSIGNED), YEAR(CURRENT_DATE())),
            1
        )
    )                                                AS published_at
FROM scopus_documents d

LEFT JOIN (
    SELECT *,
           ROW_NUMBER() OVER(
               PARTITION BY source_id, metric_year 
               ORDER BY source_metric_id DESC
           ) as rn
    FROM scopus_source_metrics
    WHERE doc_type = 'all'
) metrics
  ON  metrics.source_id   = d.source_id
  AND metrics.rn = 1
  AND metrics.metric_year = COALESCE(
        (
          SELECT ssm_c.metric_year
          FROM   scopus_source_metrics ssm_c
          WHERE  ssm_c.source_id   = d.source_id
            AND  ssm_c.doc_type    = 'all'
            AND  ssm_c.metric_year = COALESCE(YEAR(d.cover_date), CAST(RIGHT(d.cover_display_date, 4) AS UNSIGNED))
            AND  LOWER(ssm_c.cite_score_status) = 'complete'
          LIMIT 1
        ),
        CASE
          WHEN EXISTS (
            SELECT 1
            FROM   scopus_source_metrics ssm_ip
            WHERE  ssm_ip.source_id   = d.source_id
              AND  ssm_ip.doc_type    = 'all'
              AND  ssm_ip.metric_year = COALESCE(YEAR(d.cover_date), CAST(RIGHT(d.cover_display_date, 4) AS UNSIGNED))
              AND  LOWER(ssm_ip.cite_score_status) = 'in-progress'
          )
          THEN (
            SELECT MAX(ssm_prev.metric_year)
            FROM   scopus_source_metrics ssm_prev
            WHERE  ssm_prev.source_id   = d.source_id
              AND  ssm_prev.doc_type    = 'all'
              AND  ssm_prev.metric_year < COALESCE(YEAR(d.cover_date), CAST(RIGHT(d.cover_display_date, 4) AS UNSIGNED))
              AND  LOWER(ssm_prev.cite_score_status) = 'complete'
          )
          ELSE NULL
        END
      )

UNION ALL

SELECT
    CONCAT('thaijo_', d.id) COLLATE utf8mb4_general_ci AS id,
    'thaijo' COLLATE utf8mb4_general_ci              AS source_name,
    COALESCE(d.title_th, d.title_en, 'Untitled')     AS title,
    NULL                                             AS title_en,
    COALESCE(d.abstract_th, d.abstract_en)           AS abstract,
    'faculty' COLLATE utf8mb4_general_ci             AS publication_type,
    d.year                                           AS publication_year,
    d.id                                             AS source_id,
    NULL                                             AS detail_type,
    NULL                                             AS cited_by,
    NULL                                             AS track_id,
    NULL                                             AS journal_quartile,
    NULL                                             AS journal_percentile,
    (
      SELECT j.tier FROM thaijo_journals j WHERE j.journal_id = d.journal_id LIMIT 1
    )                                                AS journal_tier,
    (
      SELECT j.name_th FROM thaijo_journals j WHERE j.journal_id = d.journal_id LIMIT 1
    )                                                AS journal_name,
    d.article_url                                    AS url,
    NULLIF(TRIM(BOTH ',' FROM REPLACE(REPLACE(REPLACE(JSON_UNQUOTE(d.keywords_json), '["', ''), '"]', ''), '","', ', ')), '') AS keywords,
    NULL                                             AS poster_url,
    NULL                                             AS group_code,
    d.date_published                                 AS published_at
FROM thaijo_documents d

UNION ALL

SELECT
    CONCAT('ai_', p.id) COLLATE utf8mb4_general_ci AS id,
    'ai_showcase' COLLATE utf8mb4_general_ci        AS source_name,
    COALESCE(p.title_th, p.title_en, 'Untitled')   AS title,
    p.title_en                                       AS title_en,
    p.abstract                                       AS abstract,
    'student' COLLATE utf8mb4_general_ci             AS publication_type,
    p.published_year                                 AS publication_year,
    p.id                                             AS source_id,
    p.project_type                                   AS detail_type,
    NULL                                             AS cited_by,
    p.track_id                                       AS track_id,
    NULL                                             AS journal_quartile,
    NULL                                             AS journal_percentile,
    NULL                                             AS journal_tier,
    NULL                                             AS journal_name,
    p.ai_showcase_link                               AS url,
    NULL                                             AS keywords,
    p.poster_url                                     AS poster_url,
    p.group_code                                     AS group_code,
    MAKEDATE(p.published_year, 1)                   AS published_at
FROM ai_showcase_projects p;

-- ============================================================
-- View : unified_search_authors
-- ============================================================
CREATE OR REPLACE VIEW unified_search_authors AS
SELECT
    CONCAT('scopus_', da.document_id) COLLATE utf8mb4_general_ci  AS unified_publication_id,
    'scopus' COLLATE utf8mb4_general_ci                           AS source_name,
    COALESCE(a.full_name, 'Unknown') COLLATE utf8mb4_general_ci   AS name,
    'author'                                                      AS role,
    da.author_seq
FROM scopus_document_authors da
JOIN scopus_authors a ON a.id = da.author_id
UNION ALL
SELECT
    CONCAT('thaijo_', da.document_id) COLLATE utf8mb4_general_ci  AS unified_publication_id,
    'thaijo' COLLATE utf8mb4_general_ci                           AS source_name,
    COALESCE(da.name_th, da.name_en, 'Unknown') COLLATE utf8mb4_general_ci AS name,
    'author'                                                      AS role,
    da.author_seq
FROM thaijo_document_authors da
UNION ALL
SELECT
    CONCAT('ai_', m.project_id) COLLATE utf8mb4_general_ci        AS unified_publication_id,
    'ai_showcase' COLLATE utf8mb4_general_ci                      AS source_name,
    COALESCE(m.name, 'Unknown') COLLATE utf8mb4_general_ci        AS name,
    m.role COLLATE utf8mb4_general_ci                             AS role,
    NULL                                                          AS author_seq
FROM ai_showcase_project_members m;
