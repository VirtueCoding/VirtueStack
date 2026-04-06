SET lock_timeout = '5s';

ALTER TABLE tasks
DROP COLUMN IF EXISTS updated_at;
