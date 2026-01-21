# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [1.2.1] - 2026-01-21

### Added

- **CLI Station Management** - c
  - `--list-stations` — displays all stations in a formatted table with ID, name, mountpoint, owner, status, listeners count, and creation date
  - `--del-station {ID}` — deletes a station by its database ID
  - `--db {path}` — allows specifying direct path to database file (useful for admin access from different user account)
- **Color-Coded Status** - Station status in CLI output uses colored indicators (green for online, red for offline)

### Changed

- **Unified Versioning System** - Version is now defined in a single source of truth (`internal/version/version.go`) instead of relying on git tags. Build script reads version from this file and injects it via ldflags. This removes dependency on external sources and git tags.

## [1.2.0] - 2026-01-14

### Fixed

- **Proxy Architecture** - Fixed audio distortion issue where one StreamReader was shared between all clients. Now each client gets its own connection to remote station.
- **Federated Stations 502 Error** - Fixed proxying of federated stations by storing and using remote server port instead of local one.
- **Metadata Updates** - Proxy now requests metadata from the correct port of the remote server.
- **Rating System** - Fixed various bugs in the rating functionality.
- **VLC Playback** - Fixed VLC compatibility by proxying streams instead of redirecting.
- **Local Station Decoding** - Fixed decoding error for local stations by using direct playback.
- **HLS Memory Leaks** - Reduced HLS buffer sizes (10/15/20 sec) to prevent memory leaks.

### Added

- **VLC/Media Player Support** - Added `/stations/stream/federated/{uuid}` endpoint for VLC and other media players.
- **MIME Type Detection** - Added MIME type support check before MSE with fallback to direct playback for MP3.
- **Now Playing Display** - Shows "Now Playing" or "No metadata" label based on track metadata availability.
- **Favicon** - Added SVG radio icon for browser tab.
- **Shareable URLs** - Added shareable station URL with copy button on station cards.
- **Mountpoint Proxy** - Added `/stations/:mountpoint` proxy endpoint for media player compatibility.
- **Buffer Protection** - MSE temporary buffer overflow protection (max 128KB).

### Changed

- **Simplified Federated URLs** - Changed from `/stream/federated/{node_address}/{mountpoint}` to `/stream/federated/{uuid}`.
- **Increased Timeouts** - Proxy timeouts increased (Dial: 60s, ResponseHeader: 30s) with 16KB buffering.
- **MSE Buffers** - Increased buffers (16KB chunks, 128KB max, 30s in memory) for high-latency networks.
- **MSE Usage** - MSE player is now only used for federated streams with supported codecs (not MP3).
- **UI Improvements** - Centered footer, replaced alert/confirm dialogs with inline notifications, reduced station URL block height.
- **Logging** - Removed verbose HLS segment download logs.

### Federation Server

- **Node Port Field** - Added `node_port` field in API `/api/stations` to fix 502 errors.
- **Pull Interval** - Changed default from 300 to 60 seconds, minimum from 60 to 30 seconds.

## [1.1.0] - 2026-01-12

### Added

- **HLS Streaming** - Added ability to play HLS streams.
- **External Streams** - Support for external stream sources (`external_stream_url`, `external_stream_type` fields).
- **Auto-Start** - Auto-start feature for external streams.
- **Database Migrations** - Automatic database migration system.

### Fixed

- **Ratings** - Fixed bug with station ratings.

## [1.0.0] - 2026-01-09

### Added

- **Initial Release** - First public release of YggRadio
- **P2P Radio Platform** - Decentralized radio streaming on Yggdrasil mesh network
- **Station Management** - Create and manage radio stations with Proof-of-Work anti-spam protection
- **Authentication** - Ed25519 cryptographic authentication and Magic Link (passwordless) option
- **Federation** - Centralized federation server support for station discovery
- **Real-time Stats** - Live listener count and "now playing" metadata
- **Social Features** - Rating and comment system for stations
- **Playlist Mode** - Sequential and shuffle playlist streaming from local files
- **Security** - Audit logging, rate limiting, Content Security Policy (CSP)
- **Modern Frontend** - Responsive React frontend with Tailwind CSS
- **Single Binary** - Embedded frontend in single executable
- **Multi-Platform** - Builds for Linux, Windows, macOS, and ARM64

[1.2.1]: https://github.com/JB-SelfCompany/yggradio/compare/v1.2.0...v1.2.1
[1.2.0]: https://github.com/JB-SelfCompany/yggradio/compare/v1.1.0...v1.2.0
[1.1.0]: https://github.com/JB-SelfCompany/yggradio/compare/v1.0.0...v1.1.0
[1.0.0]: https://github.com/JB-SelfCompany/yggradio/releases/tag/v1.0.0
