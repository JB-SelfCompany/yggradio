-- Migration 001: Version 1.1.0 - External Streaming Support
-- This migration adds support for external stream sources (HLS and direct URLs)
-- Safe to run on production (adds nullable columns and indexes)

-- Add columns for external stream sources
ALTER TABLE stations ADD COLUMN external_stream_url TEXT;
ALTER TABLE stations ADD COLUMN external_stream_type TEXT; -- 'hls' or 'direct'
ALTER TABLE stations ADD COLUMN auto_start BOOLEAN DEFAULT 1;

-- Add partial index for external URLs (only indexes non-null values, saves space)
CREATE INDEX IF NOT EXISTS idx_stations_external_url
    ON stations(external_stream_url)
    WHERE external_stream_url IS NOT NULL;

-- Add index for sorting by listener count
CREATE INDEX IF NOT EXISTS idx_stations_listeners ON stations(listeners_count DESC);

-- Add index for monitoring auto_start streams
CREATE INDEX IF NOT EXISTS idx_stations_auto_start
    ON stations(auto_start)
    WHERE external_stream_url IS NOT NULL;

-- Populate external_stream_type for backward compatibility
-- If any existing stations have external_stream_url set, default them to 'direct'
UPDATE stations
SET external_stream_type = 'direct'
WHERE external_stream_url IS NOT NULL
  AND (external_stream_type IS NULL OR external_stream_type = '');

-- Notes:
-- - auto_start defaults to 1 (true) for backward compatibility
-- - When user manually stops a stream, auto_start is set to 0 (false)
-- - When user manually starts a stream, auto_start is set to 1 (true)
--
-- Rollback instructions:
-- DROP INDEX idx_stations_external_url;
-- DROP INDEX idx_stations_listeners;
-- DROP INDEX idx_stations_auto_start;
-- (columns can remain as nullable, won't break existing code)
