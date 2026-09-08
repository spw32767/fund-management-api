-- เพิ่ม index ให้คอลัมน์ conference ของ scopus_documents
-- (คอลัมน์ถูกเพิ่มไปแล้วใน 026_20260708_add_scopus_conference_columns.sql
--  แต่ index ตกหล่นตอนย้ายไฟล์ จึงเพิ่มแยกที่นี่)
CREATE INDEX IF NOT EXISTS idx_scopus_documents_conference_country
  ON scopus_documents (conference_country);

CREATE INDEX IF NOT EXISTS idx_scopus_documents_conference_city
  ON scopus_documents (conference_city);
