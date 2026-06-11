-- ============================================================
-- View : unified_search_authors
-- รวมรายชื่อผู้เขียน/สมาชิกจากทุกแหล่งข้อมูล
-- ============================================================
CREATE VIEW unified_search_contents AS

-- 1. SCOPUS 
SELECT 
    CONCAT('scopus_', d.id) AS id, 'scopus' AS source_name, COALESCE(d.title, 'Untitled') AS title, 
    d.abstract, 'faculty' AS publication_type, 
    COALESCE(YEAR(d.cover_date), CAST(RIGHT(d.cover_display_date, 4) AS UNSIGNED)) AS publication_year,
    d.id AS source_id, d.aggregation_type AS detail_type,
    NULL AS track_id, 
    NULLIF(UPPER(TRIM(sm.cite_score_quartile)), '') AS journal_quartile,
    sm.cite_score_percentile AS journal_percentile, NULL AS journal_tier, d.scopus_link AS url, d.authkeywords AS keywords,
    COALESCE(d.cover_date, MAKEDATE(COALESCE(YEAR(d.cover_date), CAST(RIGHT(d.cover_display_date, 4) AS UNSIGNED), YEAR(CURRENT_DATE())), 1)) AS published_at
FROM scopus_documents d
LEFT JOIN scopus_source_metrics sm ON sm.source_id = d.source_id AND sm.doc_type = 'all' AND sm.metric_year = COALESCE(YEAR(d.cover_date), CAST(RIGHT(d.cover_display_date, 4) AS UNSIGNED))

UNION ALL

-- 2. THAIJO 
SELECT 
    CONCAT('thaijo_', d.id) AS id, 'thaijo' AS source_name, COALESCE(d.title_th, d.title_en, 'Untitled') AS title, 
    NULL AS abstract, 'faculty' AS publication_type, d.year AS publication_year,
    d.id AS source_id, NULL AS detail_type,
    NULL AS track_id, -- ThaiJo ไม่มีคณะ
    NULL AS journal_quartile, NULL AS journal_percentile,
    (SELECT j.tier FROM thaijo_journals j WHERE j.id = d.journal_id LIMIT 1) AS journal_tier,
    d.article_url AS url, NULL AS keywords, d.date_published AS published_at
FROM thaijo_documents d

UNION ALL

-- 3. AI SHOWCASE 
SELECT 
    CONCAT('ai_', p.id) AS id, 'ai_showcase' AS source_name, COALESCE(p.title_th, p.title_en, 'Untitled') AS title, 
    p.abstract, 'student' AS publication_type, p.published_year AS publication_year,
    p.id AS source_id, p.project_type AS detail_type,
    p.track_id AS track_id,  
    NULL AS journal_quartile, NULL AS journal_percentile, NULL AS journal_tier,
    p.ai_showcase_link AS url, NULL AS keywords,
    MAKEDATE(p.published_year, 1) AS published_at
FROM ai_showcase_projects p;