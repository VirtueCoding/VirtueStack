-- Add phone column to customers table for profile updates

ALTER TABLE customers ADD COLUMN IF NOT EXISTS phone VARCHAR(20);