-- Migration 002: Add source_node_port to federated_station_cache
-- This migration adds support for storing the port of federated nodes
-- so that proxy connections can connect to the correct port.

-- Add source_node_port column (default to 8080 for existing entries)
ALTER TABLE federated_station_cache ADD COLUMN source_node_port INTEGER DEFAULT 8080;

-- Create index for UUID lookup (for simplified URL routing)
CREATE INDEX IF NOT EXISTS idx_federated_cache_uuid ON federated_station_cache(uuid);
