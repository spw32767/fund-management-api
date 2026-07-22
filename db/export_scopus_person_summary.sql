-- Export: Research Dashboard > สรุปรายบุคคล (Person Summary)
-- Target: MySQL 8+ / MariaDB 10.2+
--
-- วิธีใช้ใน phpMyAdmin
-- 1. แก้ค่าตัวกรองด้านล่างให้ตรงกับค่าบนหน้า Dashboard
-- 2. รัน script ทั้งไฟล์ในแท็บ SQL
-- 3. script จะสร้างตาราง export_scopus_person_summary
-- 4. คลิกชื่อตารางนี้ทางเมนูซ้าย แล้วเลือกแท็บ Export > CSV
--
-- หมายเหตุ
-- - ปีรับได้ทั้ง พ.ศ. (เช่น 2567) และ ค.ศ. (เช่น 2024)
-- - NULL หรือ '' หมายถึงไม่ใช้ตัวกรองนั้น
-- - ค่าหลายค่าให้คั่นด้วย comma โดยไม่ใส่ช่องว่าง เช่น 'Journal,Conference Proceeding'
-- - script สร้าง/แทนที่เฉพาะตาราง export_scopus_person_summary เท่านั้น
-- - เมื่อต้องการลบตาราง export ให้รัน: DROP TABLE export_scopus_person_summary;

SET @year_start_be := NULL;
SET @year_end_be := NULL;
SET @aggregation_types_csv := '';
SET @quality_buckets_csv := '';

-- ตัวกรองเสริม (หน้า UI ปัจจุบันอาจไม่ได้เปิดให้แสดงทุกตัว)
SET @open_access_mode := 'all'; -- all | oa | non_oa
SET @citation_min := NULL;
SET @citation_max := NULL;
SET @search_title := '';
SET @search_doi := '';
SET @search_eid := '';
SET @search_document_scopus_id := '';
SET @search_journal := '';
SET @search_author := '';
SET @search_affiliation := '';
SET @search_keyword := '';

-- ช่องค้นหาใน section Person Summary (ชื่อ / อีเมล / Scopus ID ของบุคคล)
SET @person_search := '';

CREATE OR REPLACE TABLE export_scopus_person_summary AS
SELECT *
FROM (
WITH document_rows AS (
  SELECT
    u.user_id,
    TRIM(CONCAT(COALESCE(u.user_fname, ''), ' ', COALESCE(u.user_lname, ''))) AS user_name,
    COALESCE(NULLIF(TRIM(u.email), ''), '-') AS user_email,
    COALESCE(NULLIF(TRIM(u.scopus_id), ''), '-') AS user_scopus_id,
    sd.id AS document_id,
    COALESCE(
      YEAR(sd.cover_date),
      CAST(RIGHT(sd.cover_display_date, 4) AS UNSIGNED)
    ) AS publication_year_ce,
    COALESCE(sd.citedby_count, 0) AS cited_by_count,
    COALESCE(
      NULLIF(UPPER(TRIM(metrics.cite_score_quartile)), ''),
      'N/A'
    ) AS quartile,
    metrics.cite_score_percentile,
    COALESCE(NULLIF(TRIM(sd.aggregation_type), ''), 'N/A') AS aggregation_type
  FROM users AS u
  JOIN scopus_authors AS sa
    ON TRIM(u.scopus_id) = sa.scopus_author_id
  JOIN scopus_document_authors AS sda
    ON sda.author_id = sa.id
  JOIN scopus_affiliations AS own_aff
    ON own_aff.id = sda.affiliation_id
  JOIN scopus_documents AS sd
    ON sd.id = sda.document_id
  LEFT JOIN scopus_source_metrics AS metrics
    ON metrics.source_id = sd.source_id
   AND metrics.doc_type = 'all'
   AND metrics.metric_year = COALESCE(
      (
        SELECT ssm_complete.metric_year
        FROM scopus_source_metrics AS ssm_complete
        WHERE ssm_complete.source_id = sd.source_id
          AND ssm_complete.doc_type = 'all'
          AND ssm_complete.metric_year = COALESCE(
            YEAR(sd.cover_date),
            CAST(RIGHT(sd.cover_display_date, 4) AS UNSIGNED)
          )
          AND LOWER(ssm_complete.cite_score_status) = 'complete'
        LIMIT 1
      ),
      (
        SELECT MAX(ssm_previous.metric_year)
        FROM scopus_source_metrics AS ssm_previous
        WHERE ssm_previous.source_id = sd.source_id
          AND ssm_previous.doc_type = 'all'
          AND ssm_previous.metric_year < COALESCE(
            YEAR(sd.cover_date),
            CAST(RIGHT(sd.cover_display_date, 4) AS UNSIGNED)
          )
          AND LOWER(ssm_previous.cite_score_status) = 'complete'
      )
   )
  WHERE u.delete_at IS NULL
    AND u.scopus_id IS NOT NULL
    AND TRIM(u.scopus_id) <> ''

    -- บุคคลต้องผูกกับผลงานด้วย affiliation ที่ Dashboard รองรับ
    AND LOWER(TRIM(COALESCE(own_aff.name, ''))) IN (
      'khon kaen university',
      'faculty of science, khon kaen university'
    )

    -- ช่วงปี
    AND (
      @year_start_be IS NULL
      OR COALESCE(YEAR(sd.cover_date), CAST(RIGHT(sd.cover_display_date, 4) AS UNSIGNED)) >=
         CASE WHEN @year_start_be >= 2400 THEN @year_start_be - 543 ELSE @year_start_be END
    )
    AND (
      @year_end_be IS NULL
      OR COALESCE(YEAR(sd.cover_date), CAST(RIGHT(sd.cover_display_date, 4) AS UNSIGNED)) <=
         CASE WHEN @year_end_be >= 2400 THEN @year_end_be - 543 ELSE @year_end_be END
    )

    -- ประเภทผลงาน
    AND (
      @aggregation_types_csv = ''
      OR FIND_IN_SET(sd.aggregation_type, @aggregation_types_csv) > 0
    )

    -- Open Access
    AND (
      @open_access_mode = 'all'
      OR (
        @open_access_mode = 'oa'
        AND (COALESCE(sd.openaccess_flag, 0) = 1 OR COALESCE(sd.openaccess, 0) = 1)
      )
      OR (
        @open_access_mode = 'non_oa'
        AND COALESCE(sd.openaccess_flag, 0) = 0
        AND COALESCE(sd.openaccess, 0) = 0
      )
    )

    -- Citation และช่องค้นหาระดับเอกสาร
    AND (@citation_min IS NULL OR COALESCE(sd.citedby_count, 0) >= @citation_min)
    AND (@citation_max IS NULL OR COALESCE(sd.citedby_count, 0) <= @citation_max)
    AND (@search_title = '' OR sd.title LIKE CONCAT('%', @search_title, '%'))
    AND (@search_doi = '' OR sd.doi = @search_doi)
    AND (@search_eid = '' OR sd.eid = @search_eid)
    AND (@search_document_scopus_id = '' OR sd.scopus_id = @search_document_scopus_id)
    AND (@search_journal = '' OR sd.publication_name LIKE CONCAT('%', @search_journal, '%'))
    AND (@search_keyword = '' OR sd.authkeywords LIKE CONCAT('%', @search_keyword, '%'))
    AND (
      @search_author = ''
      OR EXISTS (
        SELECT 1
        FROM scopus_document_authors AS author_sda
        JOIN scopus_authors AS author_sa
          ON author_sa.id = author_sda.author_id
        WHERE author_sda.document_id = sd.id
          AND (
            author_sa.full_name LIKE CONCAT('%', @search_author, '%')
            OR author_sa.given_name LIKE CONCAT('%', @search_author, '%')
            OR author_sa.surname LIKE CONCAT('%', @search_author, '%')
          )
      )
    )
    AND (
      @search_affiliation = ''
      OR EXISTS (
        SELECT 1
        FROM scopus_document_authors AS aff_sda
        JOIN scopus_affiliations AS search_aff
          ON search_aff.id = aff_sda.affiliation_id
        WHERE aff_sda.document_id = sd.id
          AND (
            search_aff.name LIKE CONCAT('%', @search_affiliation, '%')
            OR search_aff.city LIKE CONCAT('%', @search_affiliation, '%')
            OR search_aff.country LIKE CONCAT('%', @search_affiliation, '%')
            OR search_aff.afid LIKE CONCAT('%', @search_affiliation, '%')
          )
      )
    )

    -- T1 แยกออกจาก Q1 และ Conference ไม่นับเข้า tier ใด ๆ
    AND (
      @quality_buckets_csv = ''
      OR (
        LOWER(TRIM(COALESCE(sd.aggregation_type, ''))) <> 'conference proceeding'
        AND (
          (
            FIND_IN_SET('T1', @quality_buckets_csv) > 0
            AND metrics.cite_score_percentile BETWEEN 90 AND 100
          )
          OR (
            (metrics.cite_score_percentile IS NULL OR metrics.cite_score_percentile NOT BETWEEN 90 AND 100)
            AND FIND_IN_SET(
              COALESCE(NULLIF(UPPER(TRIM(metrics.cite_score_quartile)), ''), 'N/A'),
              @quality_buckets_csv
            ) > 0
          )
        )
      )
    )
),
raw_row_counts AS (
  SELECT
    user_id,
    COUNT(*) AS publication_rows
  FROM document_rows
  GROUP BY user_id
),
unique_person_documents AS (
  -- API นับ publication_rows ก่อน แล้ว deduplicate ด้วย user_id + document_id
  SELECT
    user_id,
    MAX(user_name) AS user_name,
    MAX(user_email) AS user_email,
    MAX(user_scopus_id) AS user_scopus_id,
    document_id,
    MAX(publication_year_ce) AS publication_year_ce,
    MAX(cited_by_count) AS cited_by_count,
    MAX(quartile) AS quartile,
    MAX(cite_score_percentile) AS cite_score_percentile,
    MAX(aggregation_type) AS aggregation_type
  FROM document_rows
  GROUP BY user_id, document_id
),
person_summary AS (
  SELECT
    d.user_id,
    d.user_name,
    d.user_email,
    d.user_scopus_id,
    r.publication_rows,
    COUNT(*) AS unique_documents,
    SUM(d.cited_by_count) AS cited_by_total,
    ROUND(SUM(d.cited_by_count) / NULLIF(COUNT(*), 0), 2) AS avg_cited_by,
    SUM(
      CASE
        WHEN LOWER(TRIM(d.aggregation_type)) <> 'conference proceeding'
         AND d.cite_score_percentile BETWEEN 90 AND 100
        THEN 1 ELSE 0
      END
    ) AS t1_count,
    SUM(
      CASE
        WHEN LOWER(TRIM(d.aggregation_type)) <> 'conference proceeding'
         AND (d.cite_score_percentile IS NULL OR d.cite_score_percentile NOT BETWEEN 90 AND 100)
         AND d.quartile = 'Q1'
        THEN 1 ELSE 0
      END
    ) AS q1_count,
    SUM(
      CASE
        WHEN LOWER(TRIM(d.aggregation_type)) <> 'conference proceeding'
         AND (d.cite_score_percentile IS NULL OR d.cite_score_percentile NOT BETWEEN 90 AND 100)
         AND d.quartile = 'Q2'
        THEN 1 ELSE 0
      END
    ) AS q2_count,
    SUM(
      CASE
        WHEN LOWER(TRIM(d.aggregation_type)) <> 'conference proceeding'
         AND (d.cite_score_percentile IS NULL OR d.cite_score_percentile NOT BETWEEN 90 AND 100)
         AND d.quartile = 'Q3'
        THEN 1 ELSE 0
      END
    ) AS q3_count,
    SUM(
      CASE
        WHEN LOWER(TRIM(d.aggregation_type)) <> 'conference proceeding'
         AND (d.cite_score_percentile IS NULL OR d.cite_score_percentile NOT BETWEEN 90 AND 100)
         AND d.quartile = 'Q4'
        THEN 1 ELSE 0
      END
    ) AS q4_count,
    SUM(
      CASE
        WHEN LOWER(TRIM(d.aggregation_type)) <> 'conference proceeding'
         AND (d.cite_score_percentile IS NULL OR d.cite_score_percentile NOT BETWEEN 90 AND 100)
         AND d.quartile NOT IN ('Q1', 'Q2', 'Q3', 'Q4')
        THEN 1 ELSE 0
      END
    ) AS quartile_na,
    SUM(CASE WHEN LOWER(TRIM(d.aggregation_type)) = 'journal' THEN 1 ELSE 0 END) AS journal_count,
    SUM(CASE WHEN LOWER(TRIM(d.aggregation_type)) IN ('book', 'book series') THEN 1 ELSE 0 END) AS book_count,
    SUM(CASE WHEN LOWER(TRIM(d.aggregation_type)) = 'conference proceeding' THEN 1 ELSE 0 END) AS conference_count,
    CASE
      WHEN MIN(CASE WHEN d.publication_year_ce > 0 THEN d.publication_year_ce END) IS NULL THEN 0
      ELSE MIN(CASE WHEN d.publication_year_ce > 0 THEN d.publication_year_ce END) + 543
    END AS first_year,
    CASE
      WHEN MAX(CASE WHEN d.publication_year_ce > 0 THEN d.publication_year_ce END) IS NULL THEN 0
      ELSE MAX(CASE WHEN d.publication_year_ce > 0 THEN d.publication_year_ce END) + 543
    END AS latest_year,
    COUNT(DISTINCT CASE WHEN d.publication_year_ce > 0 THEN d.publication_year_ce END) AS active_years
  FROM unique_person_documents AS d
  JOIN raw_row_counts AS r
    ON r.user_id = d.user_id
  GROUP BY
    d.user_id,
    d.user_name,
    d.user_email,
    d.user_scopus_id,
    r.publication_rows
),
filtered_summary AS (
  SELECT *
  FROM person_summary
  WHERE @person_search = ''
     OR LOWER(user_name) LIKE CONCAT('%', LOWER(@person_search), '%')
     OR LOWER(user_email) LIKE CONCAT('%', LOWER(@person_search), '%')
     OR LOWER(user_scopus_id) LIKE CONCAT('%', LOWER(@person_search), '%')
)
SELECT
  ROW_NUMBER() OVER (
    ORDER BY
      t1_count DESC,
      q1_count DESC,
      q2_count DESC,
      q3_count DESC,
      q4_count DESC,
      conference_count DESC,
      user_name ASC
  ) AS `ลำดับ`,
  user_name AS `ชื่อ-สกุล`,
  user_email AS `อีเมล`,
  user_scopus_id AS `Scopus ID`,
  publication_rows AS `จำนวนแถวผลงาน`,
  unique_documents AS `ผลงานไม่ซ้ำ`,
  cited_by_total AS `Citation รวม`,
  avg_cited_by AS `Citation เฉลี่ย/ผลงาน`,
  t1_count AS `T1`,
  q1_count AS `Q1`,
  q2_count AS `Q2`,
  q3_count AS `Q3`,
  q4_count AS `Q4`,
  quartile_na AS `N/A`,
  journal_count AS `Journal`,
  book_count AS `Book/Book Series`,
  conference_count AS `Conference`,
  NULLIF(first_year, 0) AS `ปีแรก`,
  NULLIF(latest_year, 0) AS `ปีล่าสุด`,
  active_years AS `จำนวนปีที่มีผลงาน`
FROM filtered_summary
) AS export_rows;

-- แสดงผลหลังสร้างตาราง (เรียงเหมือนหน้า Dashboard)
SELECT *
FROM export_scopus_person_summary
ORDER BY `ลำดับ` ASC;
