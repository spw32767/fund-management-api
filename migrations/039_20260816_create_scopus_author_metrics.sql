-- Author-level metrics จาก Scopus Author Retrieval API (content/author/author_id/{id}?view=METRICS)
-- ดึง h-index ของอาจารย์ในระบบ โดยอิง scopus_id ในตาราง users
-- Scopus คืน h-index มาเป็น "ค่าเดียว ณ ปัจจุบัน" ไม่มีประวัติย้อนหลัง
-- จึงเก็บแบบ snapshot append-only: ค่าปัจจุบัน = แถวล่าสุด, กราฟย้อนหลัง = ทุกแถวของ author เรียงตามวันที่
-- คนละระดับกับ scopus_documents/scopus_authors (metric ต่อ author ไม่ใช่ต่อ document)

CREATE TABLE IF NOT EXISTS scopus_author_metrics (
  id                BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  user_id           INT          DEFAULT NULL COMMENT 'users.user_id เจ้าของ scopus_id (nullable เผื่อ author ไม่ผูก user)',
  scopus_author_id  VARCHAR(32)  NOT NULL COMMENT 'Scopus Author ID (AU-ID) จาก users.scopus_id',
  h_index           INT          DEFAULT NULL COMMENT 'h-index ณ วันที่ยิง',
  document_count    INT          DEFAULT NULL COMMENT 'coredata document-count',
  cited_by_count    INT          DEFAULT NULL COMMENT 'coredata cited-by-count (จำนวนเอกสารที่อ้างถึง)',
  citation_count    INT          DEFAULT NULL COMMENT 'coredata citation-count (จำนวนครั้งที่ถูกอ้างรวม)',
  coauthor_count    INT          DEFAULT NULL COMMENT 'coauthor-count',
  snapshot_date     DATE         NOT NULL COMMENT 'วันที่เก็บ snapshot (กันซ้ำ 1 แถว/author/วัน)',
  raw_json          JSON         DEFAULT NULL COMMENT 'response ดิบจาก Author API เผื่อ backfill field ใหม่',
  fetched_at        DATETIME     NOT NULL COMMENT 'เวลาที่ยิง API จริง',
  created_at        DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at        DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (id),
  UNIQUE KEY uq_scopus_author_metrics_author_date (scopus_author_id, snapshot_date),
  KEY idx_scopus_author_metrics_user (user_id),
  KEY idx_scopus_author_metrics_author (scopus_author_id),
  KEY idx_scopus_author_metrics_snapshot (snapshot_date)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
