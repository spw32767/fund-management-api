-- ============================================================
-- Index: ช่วย performance สำหรับ subquery ใน Scopus Metrics
-- ============================================================
CREATE INDEX IF NOT EXISTS idx_metrics_lookup
  ON scopus_source_metrics(source_id, doc_type, metric_year, cite_score_status);

-- ============================================================
-- View : unified_search_contents
-- รวมผลงานจากทุกแหล่งข้อมูลเพื่อการค้นหาและตัวกรอง
-- ============================================================
CREATE OR REPLACE VIEW unified_search_contents AS

-- =======================
-- 1. SCOPUS
-- =======================
SELECT
    CONCAT('scopus_', d.id) COLLATE utf8mb4_general_ci AS id,
    'scopus' COLLATE utf8mb4_unicode_ci             AS source_name,
    COALESCE(d.title, 'Untitled')                   AS title,
    d.abstract                                       AS abstract,
    'faculty'                                        AS publication_type,
    COALESCE(
      YEAR(d.cover_date),
      CAST(RIGHT(d.cover_display_date, 4) AS UNSIGNED)
    )                                                AS publication_year,
    d.id                                             AS source_id,
    d.aggregation_type                               AS detail_type,
    NULL                                             AS track_id,
    d.citedby_count                                  AS cited_by,
  
    CASE WHEN d.aggregation_type = 'Conference Proceeding' THEN NULL ELSE NULLIF(UPPER(TRIM(metrics.cite_score_quartile)), '') END AS journal_quartile,
      
    metrics.cite_score_percentile                    AS journal_percentile,
    NULL                                             AS journal_tier,
    d.scopus_link                                    AS url,
    d.authkeywords                                   AS keywords,
  
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
            AND  ssm_c.metric_year = COALESCE(
                   YEAR(d.cover_date),
                   CAST(RIGHT(d.cover_display_date, 4) AS UNSIGNED)
                 )
            AND  LOWER(ssm_c.cite_score_status) = 'complete'
          LIMIT 1
        ),
        CASE
          WHEN EXISTS (
            SELECT 1
            FROM   scopus_source_metrics ssm_ip
            WHERE  ssm_ip.source_id   = d.source_id
              AND  ssm_ip.doc_type    = 'all'
              AND  ssm_ip.metric_year = COALESCE(
                     YEAR(d.cover_date),
                     CAST(RIGHT(d.cover_display_date, 4) AS UNSIGNED)
                   )
              AND  LOWER(ssm_ip.cite_score_status) = 'in-progress'
          )
          THEN (
            SELECT MAX(ssm_prev.metric_year)
            FROM   scopus_source_metrics ssm_prev
            WHERE  ssm_prev.source_id   = d.source_id
              AND  ssm_prev.doc_type    = 'all'
              AND  ssm_prev.metric_year < COALESCE(
                     YEAR(d.cover_date),
                     CAST(RIGHT(d.cover_display_date, 4) AS UNSIGNED)
                   )
              AND  LOWER(ssm_prev.cite_score_status) = 'complete'
          )
          ELSE NULL
        END
      )

UNION ALL

-- =======================
-- 2. THAIJO 
-- =======================
SELECT
    CONCAT('thaijo_', d.id) COLLATE utf8mb4_general_ci AS id,
    'thaijo' COLLATE utf8mb4_unicode_ci              AS source_name,
    COALESCE(d.title_th, d.title_en, 'Untitled')     AS title,
    COALESCE(d.abstract_th, d.abstract_en)           AS abstract,
    'faculty'                                        AS publication_type,
    d.year                                           AS publication_year,
    d.id                                             AS source_id,
    NULL                                             AS detail_type,
    NULL                                             AS track_id,
    NULL                                             AS cited_by,
    NULL                                             AS journal_quartile,
    NULL                                             AS journal_percentile,
    (
      SELECT j.tier
      FROM   thaijo_journals j
      WHERE  j.journal_id = d.journal_id
      LIMIT  1
    )                                                AS journal_tier,
    d.article_url                                    AS url,
    
    NULLIF(
      TRIM(BOTH ',' FROM REPLACE(
        REPLACE(
          REPLACE(JSON_UNQUOTE(d.keywords_json), '["', ''), 
          '"]', ''
        ), 
        '","', ', '
      )),
      ''
    )                                                AS keywords,
    
    d.date_published                                 AS published_at

FROM thaijo_documents d

UNION ALL

-- =======================
-- 3. AI SHOWCASE
-- =======================
SELECT
    CONCAT('ai_', p.id) COLLATE utf8mb4_general_ci AS id,
    'ai_showcase' COLLATE utf8mb4_unicode_ci        AS source_name,
    COALESCE(p.title_th, p.title_en, 'Untitled')     AS title,
    p.abstract                                       AS abstract,
    'student'                                        AS publication_type,
    p.published_year                                 AS publication_year,
    p.id                                             AS source_id,
    p.project_type                                   AS detail_type,
    p.track_id                                       AS track_id,
    NULL                                             AS cited_by,
    NULL                                             AS journal_quartile,
    NULL                                             AS journal_percentile,
    NULL                                             AS journal_tier,
    p.ai_showcase_link                               AS url,
    NULL                                             AS keywords,
    MAKEDATE(p.published_year, 1)                   AS published_at

FROM ai_showcase_projects p;
