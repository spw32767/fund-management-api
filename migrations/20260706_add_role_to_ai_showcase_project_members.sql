ALTER TABLE ai_showcase_project_members
  ADD COLUMN role VARCHAR(20) NOT NULL DEFAULT 'student'
  AFTER name,
  ADD KEY idx_ai_member_role (role);
