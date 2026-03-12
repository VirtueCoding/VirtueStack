-- Add version and description columns to templates table for audit trail and metadata
ALTER TABLE templates ADD COLUMN IF NOT EXISTS version INTEGER DEFAULT 1 NOT NULL;
ALTER TABLE templates ADD COLUMN IF NOT EXISTS description TEXT DEFAULT '' NOT NULL;
ALTER TABLE templates ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ DEFAULT NOW() NOT NULL;

-- Create index on version for efficient queries
CREATE INDEX IF NOT EXISTS idx_templates_version ON templates(version);

-- Add comment for documentation
COMMENT ON COLUMN templates.version IS 'Version number incremented on each update for audit trail';
COMMENT ON COLUMN templates.description IS 'Optional description of the template';