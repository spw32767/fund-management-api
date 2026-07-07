ALTER TABLE ai_showcase_project_members
  ADD COLUMN IF NOT EXISTS role VARCHAR(20) NOT NULL DEFAULT 'student'
  AFTER name,
  ADD KEY IF NOT EXISTS idx_ai_member_role (role);
