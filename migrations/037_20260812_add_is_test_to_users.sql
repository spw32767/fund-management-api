-- Add a test/system-account marker to users. The external faculty directory
-- (GET /api/ext/v1/users) returns only is_test = 0 rows, so operators can hide
-- test/dev/system accounts from external consumers without deleting them.
--
-- Flagging specific accounts is done by the operator, e.g.:
--   UPDATE users SET is_test = 1 WHERE email IN ('testteacher@cpkku.ac.th', ...);
-- This migration only adds the column (defaults everyone to 0 = not test).

ALTER TABLE users
  ADD COLUMN IF NOT EXISTS is_test tinyint(1) NOT NULL DEFAULT 0 AFTER Is_active;
