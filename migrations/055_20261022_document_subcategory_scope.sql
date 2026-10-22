-- Optional scope for generic-fund document types, configured in admin Settings.
-- NULL keeps existing behavior. Do not reuse the obsolete subcategory_ids field:
-- old installations contain legacy/test values there that are no longer applied.
ALTER TABLE document_types
  ADD COLUMN IF NOT EXISTS subcategory_scope LONGTEXT DEFAULT NULL
  CHECK (subcategory_scope IS NULL OR JSON_VALID(subcategory_scope));

-- Configure after deploying:
-- New copyright documents: fund_application + include the selected 2.9 IDs.
-- Research proposal/budget: fund_application + exclude the same 2.9 IDs.
-- Existing document rows and historical submission attachments are unchanged.
