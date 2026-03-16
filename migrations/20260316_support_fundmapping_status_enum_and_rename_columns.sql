START TRANSACTION;

-- Normalize legacy status values before converting to ENUM
UPDATE support_fundmapping
SET maching_status = UPPER(TRIM(maching_status))
WHERE maching_status IS NOT NULL;

UPDATE support_fundmapping
SET maching_status = 'N'
WHERE maching_status IS NULL
   OR TRIM(maching_status) = ''
   OR UPPER(TRIM(maching_status)) NOT IN ('N', 'Y', 'C', 'D');

-- Rename typo columns and enforce explicit status domain
ALTER TABLE support_fundmapping
  CHANGE COLUMN maching_status matching_status ENUM('N', 'Y', 'C', 'D') NOT NULL DEFAULT 'N' COMMENT 'N=ยังไม่ได้จับคู่, Y=จับคู่แล้ว, C=ปิดโครงการแล้ว, D=ยกเลิกความต้องการ',
  CHANGE COLUMN mached_researcher matched_researcher TEXT NULL COMMENT 'ชื่อนักวิจัยของ CP';

COMMIT;
