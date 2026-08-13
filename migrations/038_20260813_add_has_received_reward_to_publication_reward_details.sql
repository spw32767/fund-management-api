-- Records whether the applicant already received/requested the publication
-- reward for this article and is now requesting manuscript-related costs only.
ALTER TABLE publication_reward_details
  ADD COLUMN IF NOT EXISTS has_received_reward tinyint(1) NOT NULL DEFAULT 0 AFTER reward_amount;
