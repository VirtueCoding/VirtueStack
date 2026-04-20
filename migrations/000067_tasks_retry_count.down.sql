SET lock_timeout = '5s';

ALTER TABLE tasks
DROP COLUMN IF EXISTS retry_count;
