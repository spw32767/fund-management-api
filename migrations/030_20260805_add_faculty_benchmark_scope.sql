-- เพิ่ม scope ระดับ "คณะ" สำหรับการเทียบ
-- คณะนับด้วย author-set query: AF-ID(KKU) AND SUBJAREA(COMP) AND (AU-ID ของอาจารย์ในระบบ)
-- ใช้ได้กับ API key ที่มีสิทธิ์แค่ STANDARD (count) ไม่ต้องพึ่ง view=COMPLETE
INSERT INTO scopus_benchmark_scopes (code, label, level, af_id, affil_country, subject_area, active)
SELECT * FROM (SELECT 'faculty_cs' AS code, 'คณะ (Computer Science)' AS label, 'faculty' AS level,
                      NULL AS af_id, NULL AS affil_country, 'COMP' AS subject_area, 1 AS active) AS tmp
WHERE NOT EXISTS (SELECT 1 FROM scopus_benchmark_scopes WHERE code = 'faculty_cs');
