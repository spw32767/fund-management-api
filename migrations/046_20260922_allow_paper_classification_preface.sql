-- Run after migration 045. Preface marks a record that cannot be classified;
-- its category remains NULL and requires human review.
ALTER TABLE scopus_benchmark_documents
  MODIFY COLUMN classification_confidence ENUM('High', 'Medium', 'Low', 'Preface') DEFAULT NULL;

ALTER TABLE publication_reward_details
  MODIFY COLUMN classification_confidence ENUM('High', 'Medium', 'Low', 'Preface') DEFAULT NULL;
