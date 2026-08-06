CREATE TABLE IF NOT EXISTS submission_approval_attachments (
  attachment_id INT NOT NULL AUTO_INCREMENT,
  submission_id INT NOT NULL,
  label VARCHAR(255) NOT NULL,
  original_filename VARCHAR(255) NOT NULL,
  stored_filename VARCHAR(255) NOT NULL,
  stored_path VARCHAR(500) NOT NULL,
  mime_type VARCHAR(100) NOT NULL DEFAULT 'application/pdf',
  file_size BIGINT NOT NULL,
  file_hash VARCHAR(64) DEFAULT NULL,
  display_order INT NOT NULL DEFAULT 0,
  uploaded_by INT NOT NULL,
  uploaded_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  deleted_at DATETIME DEFAULT NULL,
  PRIMARY KEY (attachment_id),
  KEY idx_submission_approval_attachments_submission (submission_id, deleted_at, display_order),
  KEY idx_submission_approval_attachments_uploaded_by (uploaded_by),
  CONSTRAINT fk_submission_approval_attachments_submission
    FOREIGN KEY (submission_id) REFERENCES submissions (submission_id),
  CONSTRAINT fk_submission_approval_attachments_uploader
    FOREIGN KEY (uploaded_by) REFERENCES users (user_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

INSERT INTO permissions (code, resource, action, description)
VALUES (
  'submission.approval_attachment.manage',
  'submission_approval_attachment',
  'manage',
  'Manage PDF approval evidence attached to submissions'
)
ON DUPLICATE KEY UPDATE
  resource = VALUES(resource),
  action = VALUES(action),
  description = VALUES(description),
  update_at = CURRENT_TIMESTAMP;

INSERT INTO role_permissions (role_id, permission_id, create_at, update_at, delete_at)
SELECT 3, permission_id, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL
FROM permissions
WHERE code = 'submission.approval_attachment.manage'
ON DUPLICATE KEY UPDATE
  delete_at = NULL,
  update_at = CURRENT_TIMESTAMP;
