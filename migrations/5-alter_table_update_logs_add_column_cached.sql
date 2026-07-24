ALTER TABLE update_logs ADD COLUMN cached BOOLEAN NOT NULL DEFAULT 0; -- True if schedule sent from cache
