-- YggRadio Database Schema
-- SQLite configuration for security and performance

PRAGMA journal_mode=WAL;
PRAGMA foreign_keys=ON;
PRAGMA busy_timeout=5000;
PRAGMA synchronous=NORMAL;

-- Radio stations (live streaming)
CREATE TABLE IF NOT EXISTS stations (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    uuid TEXT UNIQUE NOT NULL,
    name TEXT NOT NULL,
    description TEXT,
    mountpoint TEXT UNIQUE NOT NULL,
    owner_pubkey TEXT NOT NULL,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    status TEXT DEFAULT 'offline',
    listeners_count INTEGER DEFAULT 0,
    metadata_title TEXT,
    content_type TEXT DEFAULT 'audio/mpeg',
    bitrate INTEGER,
    genre TEXT DEFAULT '',
    is_private BOOLEAN DEFAULT 0,
    playlist_enabled BOOLEAN DEFAULT 0,
    playlist_directory TEXT DEFAULT '',
    playlist_mode TEXT DEFAULT 'sequential',
    playlist_loop BOOLEAN DEFAULT 1,
    playlist_file_pattern TEXT DEFAULT '*.{mp3,ogg,opus,flac,m4a,aac,wav,wma,ape}',
    pow_hash TEXT NOT NULL,
    federation_source TEXT DEFAULT NULL,
    UNIQUE(owner_pubkey, mountpoint)
);

CREATE INDEX IF NOT EXISTS idx_stations_owner ON stations(owner_pubkey);
CREATE INDEX IF NOT EXISTS idx_stations_status ON stations(status);
CREATE INDEX IF NOT EXISTS idx_stations_mountpoint ON stations(mountpoint);
CREATE INDEX IF NOT EXISTS idx_stations_genre ON stations(genre);
CREATE INDEX IF NOT EXISTS idx_stations_private ON stations(is_private);
CREATE INDEX IF NOT EXISTS idx_stations_playlist_enabled ON stations(playlist_enabled);
CREATE INDEX IF NOT EXISTS idx_stations_federation_source ON stations(federation_source);

-- Users (anonymous, privacy-first)
-- pubkey is NULLABLE for magic link users who authenticate via session cookies
CREATE TABLE IF NOT EXISTS users (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    pubkey TEXT UNIQUE,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    is_admin BOOLEAN DEFAULT 0
);

CREATE INDEX IF NOT EXISTS idx_users_pubkey ON users(pubkey);

-- Magic link authentication tokens
-- Stores hashed tokens for passwordless authentication
CREATE TABLE IF NOT EXISTS magic_links (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    token_hash TEXT UNIQUE NOT NULL,
    user_id INTEGER NOT NULL,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    last_used DATETIME DEFAULT CURRENT_TIMESTAMP,
    is_active BOOLEAN DEFAULT 1,
    ipv6_created TEXT,
    user_agent TEXT,
    FOREIGN KEY(user_id) REFERENCES users(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_magic_links_token ON magic_links(token_hash);
CREATE INDEX IF NOT EXISTS idx_magic_links_user ON magic_links(user_id);

-- Session cookies for magic link authentication
-- Cookies are hashed before storage, never stored in plaintext
CREATE TABLE IF NOT EXISTS auth_cookies (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    cookie_hash TEXT UNIQUE NOT NULL,
    magic_link_id INTEGER NOT NULL,
    user_id INTEGER NOT NULL,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    expires_at DATETIME NOT NULL,
    last_used DATETIME DEFAULT CURRENT_TIMESTAMP,
    ipv6_address TEXT,
    user_agent TEXT,
    FOREIGN KEY(magic_link_id) REFERENCES magic_links(id) ON DELETE CASCADE,
    FOREIGN KEY(user_id) REFERENCES users(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_auth_cookies_hash ON auth_cookies(cookie_hash);
CREATE INDEX IF NOT EXISTS idx_auth_cookies_expires ON auth_cookies(expires_at);

-- Comments
CREATE TABLE IF NOT EXISTS comments (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    uuid TEXT UNIQUE NOT NULL,
    target_type TEXT NOT NULL,
    target_id INTEGER NOT NULL,
    author_pubkey TEXT NOT NULL,
    content TEXT NOT NULL,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    parent_id INTEGER,
    is_deleted BOOLEAN DEFAULT 0,
    FOREIGN KEY(parent_id) REFERENCES comments(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_comments_target ON comments(target_type, target_id);
CREATE INDEX IF NOT EXISTS idx_comments_author ON comments(author_pubkey);
CREATE INDEX IF NOT EXISTS idx_comments_created ON comments(created_at DESC);

-- Ratings
CREATE TABLE IF NOT EXISTS ratings (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    target_type TEXT NOT NULL,
    target_id INTEGER NOT NULL,
    user_pubkey TEXT NOT NULL,
    user_ipv6 TEXT NOT NULL,
    user_ipv6_subnet TEXT NOT NULL,
    rating INTEGER NOT NULL CHECK(rating >= 1 AND rating <= 5),
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(target_type, target_id, user_pubkey)
);

CREATE INDEX IF NOT EXISTS idx_ratings_target ON ratings(target_type, target_id);
CREATE INDEX IF NOT EXISTS idx_ratings_ipv6 ON ratings(target_type, target_id, user_ipv6);
CREATE INDEX IF NOT EXISTS idx_ratings_subnet ON ratings(target_type, target_id, user_ipv6_subnet);

-- Federation: Federated station cache
CREATE TABLE IF NOT EXISTS federated_station_cache (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    uuid TEXT UNIQUE NOT NULL,
    source_node TEXT NOT NULL,
    source_node_address TEXT,
    source_node_name TEXT,
    name TEXT NOT NULL,
    description TEXT,
    mountpoint TEXT NOT NULL,
    owner_pubkey TEXT NOT NULL,
    status TEXT NOT NULL,
    listeners_count INTEGER DEFAULT 0,
    content_type TEXT,
    bitrate INTEGER,
    genre TEXT,
    metadata_title TEXT,
    average_rating REAL DEFAULT 0,
    vote_count INTEGER DEFAULT 0,
    cached_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    last_updated DATETIME DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(source_node, mountpoint)
);

CREATE INDEX IF NOT EXISTS idx_federated_cache_source ON federated_station_cache(source_node);
CREATE INDEX IF NOT EXISTS idx_federated_cache_status ON federated_station_cache(status);
CREATE INDEX IF NOT EXISTS idx_federated_cache_genre ON federated_station_cache(genre);
CREATE INDEX IF NOT EXISTS idx_federated_cache_updated ON federated_station_cache(last_updated);

-- Legacy federation tables - REMOVED in Phase 2
-- DROP TABLE IF EXISTS federation_instances;
-- DROP TABLE IF EXISTS cache_chunks;

-- Security audit log
CREATE TABLE IF NOT EXISTS security_audit_log (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    event_type TEXT NOT NULL,
    severity TEXT NOT NULL,
    ipv6_address TEXT,
    pubkey TEXT,
    endpoint TEXT,
    details TEXT,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_audit_event ON security_audit_log(event_type);
CREATE INDEX IF NOT EXISTS idx_audit_severity ON security_audit_log(severity);
CREATE INDEX IF NOT EXISTS idx_audit_created ON security_audit_log(created_at DESC);
CREATE INDEX IF NOT EXISTS idx_audit_pubkey ON security_audit_log(pubkey);

-- Session tokens for authentication caching
CREATE TABLE IF NOT EXISTS auth_sessions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    session_token TEXT UNIQUE NOT NULL,
    pubkey TEXT NOT NULL,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    expires_at DATETIME NOT NULL,
    last_used DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_sessions_token ON auth_sessions(session_token);
CREATE INDEX IF NOT EXISTS idx_sessions_pubkey ON auth_sessions(pubkey);
CREATE INDEX IF NOT EXISTS idx_sessions_expires ON auth_sessions(expires_at);

-- Server configuration key-value store
-- Used for tracking federation state changes
CREATE TABLE IF NOT EXISTS server_config (
    key TEXT PRIMARY KEY,
    value TEXT NOT NULL,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_server_config_key ON server_config(key);
