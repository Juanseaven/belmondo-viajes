-- Keep webhook media only until it reaches the configured blob store. This
-- makes retries survive backend restarts without retaining duplicate binaries.
ALTER TABLE media_assets ADD COLUMN source_data bytea;
ALTER TABLE media_assets ADD COLUMN source_url text;
