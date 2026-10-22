-- Add copyright request documents for configuration in the existing admin UI.
-- Leave fund_types NULL (inactive) until an admin chooses where to use them.
-- The existing UI scopes documents by fund_application/publication_reward only;
-- enabling fund_application applies to ALL generic funds, not only fund 2.9.
-- Existing upload forms still accept PDF only; Word support is not added here.
-- No existing document types or historical evidence are renamed/deleted.
-- Safe to rerun: match by stable code rather than environment-specific IDs.
START TRANSACTION;

INSERT INTO document_types
  (document_type_name, code, required, multiple, document_order, fund_types, create_at, update_at)
SELECT checklist.name, checklist.code, 1, 0, checklist.sort_order,
       NULL, CURRENT_TIMESTAMP(), CURRENT_TIMESTAMP()
FROM (
  SELECT 'หนังสือแจ้งความประสงค์ยื่นคำขอ' AS name,
         'copyright_intent_letter' AS code, 1 AS sort_order
  UNION ALL SELECT 'แบบฟอร์มแจ้งข้อมูลลิขสิทธิ์', 'copyright_notification_form', 2
  UNION ALL SELECT 'ผลงานที่ใช้ในการยื่นคำขอ', 'copyright_work', 3
  UNION ALL SELECT 'สำเนาบัตรผู้สร้างสรรค์ทุกคน', 'copyright_creator_ids', 4
) AS checklist
WHERE NOT EXISTS (
  SELECT 1 FROM document_types existing WHERE existing.code = checklist.code
);

COMMIT;
