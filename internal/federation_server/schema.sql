-- YggRadio Federation Server Database Schema
-- This schema tracks registered YggRadio nodes and aggregated station data

-- Table: federation_nodes
-- Tracks all registered YggRadio nodes
CREATE TABLE IF NOT EXISTS federation_nodes (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    uuid TEXT UNIQUE NOT NULL,
    address TEXT NOT NULL,
    port INTEGER NOT NULL,
    pubkey TEXT UNIQUE NOT NULL,
    name TEXT NOT NULL,
    description TEXT,
    version TEXT,
    first_registered DATETIME DEFAULT CURRENT_TIMESTAMP,
    last_seen DATETIME DEFAULT CURRENT_TIMESTAMP,
    last_pull_attempt DATETIME,
    last_pull_success DATETIME,
    status TEXT DEFAULT 'pending' CHECK(status IN ('pending', 'active', 'offline', 'blocked')),
    consecutive_failures INTEGER DEFAULT 0,
    total_pulls INTEGER DEFAULT 0,
    total_stations INTEGER DEFAULT 0
);

-- Indexes for federation_nodes
CREATE INDEX IF NOT EXISTS idx_federation_nodes_pubkey ON federation_nodes(pubkey);
CREATE INDEX IF NOT EXISTS idx_federation_nodes_status ON federation_nodes(status);
CREATE INDEX IF NOT EXISTS idx_federation_nodes_last_seen ON federation_nodes(last_seen);

-- Table: federated_stations
-- Aggregated station data from all nodes (includes rating information)
CREATE TABLE IF NOT EXISTS federated_stations (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    uuid TEXT NOT NULL,
    node_uuid TEXT NOT NULL,
    name TEXT NOT NULL,
    description TEXT,
    mountpoint TEXT NOT NULL,
    owner_pubkey TEXT NOT NULL,
    status TEXT NOT NULL CHECK(status IN ('online', 'offline')),
    listeners_count INTEGER DEFAULT 0,
    content_type TEXT,
    bitrate INTEGER,
    genre TEXT,
    metadata_title TEXT,
    average_rating REAL DEFAULT 0,
    vote_count INTEGER DEFAULT 0,
    created_at DATETIME,
    updated_at DATETIME,
    first_seen DATETIME DEFAULT CURRENT_TIMESTAMP,
    last_seen DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY(node_uuid) REFERENCES federation_nodes(uuid) ON DELETE CASCADE,
    UNIQUE(node_uuid, mountpoint)
);

-- Indexes for federated_stations
CREATE INDEX IF NOT EXISTS idx_federated_stations_node_uuid ON federated_stations(node_uuid);
CREATE INDEX IF NOT EXISTS idx_federated_stations_status ON federated_stations(status);
CREATE INDEX IF NOT EXISTS idx_federated_stations_genre ON federated_stations(genre);
CREATE INDEX IF NOT EXISTS idx_federated_stations_last_seen ON federated_stations(last_seen);
CREATE INDEX IF NOT EXISTS idx_federated_stations_rating ON federated_stations(average_rating);

-- Table: pull_history
-- Audit trail for all pull operations from nodes
CREATE TABLE IF NOT EXISTS pull_history (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    node_uuid TEXT NOT NULL,
    pull_timestamp DATETIME DEFAULT CURRENT_TIMESTAMP,
    success BOOLEAN NOT NULL,
    stations_pulled INTEGER DEFAULT 0,
    error_message TEXT,
    duration_ms INTEGER,
    FOREIGN KEY(node_uuid) REFERENCES federation_nodes(uuid) ON DELETE CASCADE
);

-- Indexes for pull_history
CREATE INDEX IF NOT EXISTS idx_pull_history_node_uuid ON pull_history(node_uuid);
CREATE INDEX IF NOT EXISTS idx_pull_history_timestamp ON pull_history(pull_timestamp);
CREATE INDEX IF NOT EXISTS idx_pull_history_success ON pull_history(success);

-- Table: security_audit_log
-- Security events (rate limits, auth failures, etc.)
CREATE TABLE IF NOT EXISTS security_audit_log (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    event_type TEXT NOT NULL,
    severity TEXT NOT NULL CHECK(severity IN ('low', 'medium', 'high', 'critical')),
    ipv6_address TEXT,
    pubkey TEXT,
    endpoint TEXT,
    details TEXT,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- Indexes for security_audit_log
CREATE INDEX IF NOT EXISTS idx_security_audit_log_event_type ON security_audit_log(event_type);
CREATE INDEX IF NOT EXISTS idx_security_audit_log_severity ON security_audit_log(severity);
CREATE INDEX IF NOT EXISTS idx_security_audit_log_created_at ON security_audit_log(created_at);
CREATE INDEX IF NOT EXISTS idx_security_audit_log_pubkey ON security_audit_log(pubkey);
