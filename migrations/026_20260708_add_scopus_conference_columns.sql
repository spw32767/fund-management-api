-- เพิ่มคอลัมน์ข้อมูล conference ให้ scopus_documents
-- (ใช้กับเอกสารประเภท Conference Proceeding — ดูได้จาก aggregation_type LIKE '%conf%')
ALTER TABLE scopus_documents
  ADD COLUMN IF NOT EXISTS conference_name text DEFAULT NULL AFTER fund_sponsor,
  ADD COLUMN IF NOT EXISTS conference_venue text DEFAULT NULL AFTER conference_name,
  ADD COLUMN IF NOT EXISTS conference_city varchar(255) DEFAULT NULL AFTER conference_venue,
  ADD COLUMN IF NOT EXISTS conference_country varchar(64) DEFAULT NULL AFTER conference_city,
  ADD COLUMN IF NOT EXISTS conference_location text DEFAULT NULL AFTER conference_country,
  ADD COLUMN IF NOT EXISTS conference_info_json longtext CHARACTER SET utf8mb4 COLLATE utf8mb4_bin DEFAULT NULL CHECK (json_valid(conference_info_json)) AFTER conference_location,
  ADD COLUMN IF NOT EXISTS conference_info_fetched_at datetime DEFAULT NULL AFTER conference_info_json;
