-- Abstract Retrieval XML author roles for documents owned by registered faculty.
-- NULL flags mean that the role has not been resolved; co-author is derived when
-- both flags are 0 on a completed document.
ALTER TABLE scopus_documents
  ADD COLUMN author_role_status VARCHAR(32) DEFAULT NULL AFTER conference_info_fetched_at,
  ADD COLUMN author_role_checked_at DATETIME DEFAULT NULL AFTER author_role_status,
  ADD KEY idx_scopus_documents_author_role_status (author_role_status);

ALTER TABLE scopus_document_authors
  ADD COLUMN is_first_author TINYINT(1) DEFAULT NULL AFTER affiliation_id,
  ADD COLUMN is_corresponding_author TINYINT(1) DEFAULT NULL AFTER is_first_author;
