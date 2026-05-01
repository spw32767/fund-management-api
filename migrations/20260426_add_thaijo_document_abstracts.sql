ALTER TABLE thaijo_documents
  ADD COLUMN IF NOT EXISTS abstract_en longtext DEFAULT NULL AFTER title_th,
  ADD COLUMN IF NOT EXISTS abstract_th longtext DEFAULT NULL AFTER abstract_en;
