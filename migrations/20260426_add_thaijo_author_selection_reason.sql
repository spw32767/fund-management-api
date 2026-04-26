ALTER TABLE thaijo_api_import_jobs
  ADD COLUMN IF NOT EXISTS author_selection_reason varchar(64) DEFAULT NULL AFTER total_results;
