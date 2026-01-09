package streaming

import (
	"bytes"
	"fmt"
	"text/template"
)

// PlaylistScriptGenerator generates scripts for local playlist streaming
type PlaylistScriptGenerator struct {
	serverURL string
}

// NewPlaylistScriptGenerator creates a new script generator
func NewPlaylistScriptGenerator(serverURL string) *PlaylistScriptGenerator {
	return &PlaylistScriptGenerator{
		serverURL: serverURL,
	}
}

// BashScriptConfig holds configuration for bash script generation
type BashScriptConfig struct {
	PlaylistDir string
	FilePattern string
	Mode        string // "sequential" or "shuffle"
	Loop        bool
	StreamURL   string
	Username    string
	Password    string
	Bitrate     int
}

// PowerShellScriptConfig holds configuration for PowerShell script generation
type PowerShellScriptConfig struct {
	PlaylistDir string
	FilePattern string
	Mode        string // "sequential" or "shuffle"
	Loop        bool
	StreamURL   string
	Username    string
	Password    string
	Bitrate     int
}

// SystemdUnitConfig holds configuration for systemd unit generation
type SystemdUnitConfig struct {
	ServiceName string
	Description string
	ScriptPath  string
	WorkingDir  string
}

// GenerateBashScript generates a bash script for playlist streaming
func (g *PlaylistScriptGenerator) GenerateBashScript(config BashScriptConfig) (string, error) {
	tmpl := `#!/bin/bash
#
# YggRadio Playlist Streaming Script
# Generated automatically - DO NOT EDIT
#
# This script streams audio files from a local directory to YggRadio
#

set -euo pipefail

# Configuration
PLAYLIST_DIR="{{.PlaylistDir}}"
FILE_PATTERN="{{.FilePattern}}"
MODE="{{.Mode}}"
LOOP={{if .Loop}}true{{else}}false{{end}}
STREAM_URL="{{.StreamURL}}"
USERNAME="{{.Username}}"
PASSWORD="{{.Password}}"
BITRATE={{.Bitrate}}

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Logging functions (all logs go to stderr to not interfere with stdout)
log_info() {
    echo -e "${GREEN}[INFO]${NC} $1" >&2
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1" >&2
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1" >&2
}

# Check for required commands
check_dependencies() {
    log_info "Checking dependencies..."

    if ! command -v ffmpeg &> /dev/null; then
        log_error "ffmpeg is not installed. Please install it first."
        log_info "Ubuntu/Debian: sudo apt-get install ffmpeg"
        log_info "macOS: brew install ffmpeg"
        exit 1
    fi

    if [ "$MODE" = "shuffle" ] && ! command -v shuf &> /dev/null; then
        log_error "shuf command not found (required for shuffle mode)"
        exit 1
    fi

    log_info "All dependencies satisfied"
}

# Validate directory
validate_directory() {
    log_info "Validating playlist directory..."

    if [ ! -d "$PLAYLIST_DIR" ]; then
        log_error "Directory does not exist: $PLAYLIST_DIR"
        exit 1
    fi

    log_info "Directory exists: $PLAYLIST_DIR"
}

# Stream a single file
stream_file() {
    local file="$1"
    local filename=$(basename "$file")

    log_info "Now playing: $filename"

    # Extract song name from filename (remove extension)
    local song_name=$(echo "$filename" | sed 's/\.[^.]*$//')

    # Update metadata on server
    curl -s -X GET "${STREAM_URL}?mode=updinfo&song=${song_name}" > /dev/null 2>&1 || {
        log_warn "Failed to update metadata (continuing anyway)"
    }

    # Brief pause to ensure metadata is updated before streaming starts
    sleep 0.5

    # Stream the file (MP3 only)
    # Build URL with embedded credentials
    local auth_url="${STREAM_URL/http:\/\//http://${USERNAME}:${PASSWORD}@}"

    ffmpeg -re -i "$file" \
        -codec:a libmp3lame \
        -b:a ${BITRATE}k \
        -vn \
        -f mp3 \
        -content_type audio/mpeg \
        -method PUT \
        "$auth_url" 2>&1 | grep -E "(Opening|error)" || {
            log_warn "Failed to stream: $filename (skipping)"
            return 1
        }

    log_info "Finished: $filename"
}

# Get file list based on mode
get_file_list() {
    local files=()

    log_info "Scanning for files matching pattern: $FILE_PATTERN"

    # Find files matching pattern (supports brace expansion for multiple extensions)
    # Recursively search in all subdirectories
    if [[ "$FILE_PATTERN" == *"{"*"}"* ]]; then
        # Pattern contains brace expansion (e.g., *.{mp3,ogg,flac})
        # Search for all common audio formats recursively
        while IFS= read -r -d '' file; do
            files+=("$file")
        done < <(find "$PLAYLIST_DIR" -type f \( \
            -iname "*.mp3" -o \
            -iname "*.ogg" -o \
            -iname "*.opus" -o \
            -iname "*.flac" -o \
            -iname "*.m4a" -o \
            -iname "*.aac" -o \
            -iname "*.wav" -o \
            -iname "*.wma" -o \
            -iname "*.ape" \
        \) -print0)
    else
        # Simple pattern without braces (also recursive)
        while IFS= read -r -d '' file; do
            files+=("$file")
        done < <(find "$PLAYLIST_DIR" -type f -iname "$FILE_PATTERN" -print0)
    fi

    if [ ${#files[@]} -eq 0 ]; then
        log_error "No files found matching pattern: $FILE_PATTERN"
        exit 1
    fi

    log_info "Found ${#files[@]} file(s)"

    # Sort or shuffle based on mode
    if [ "$MODE" = "shuffle" ]; then
        log_info "Shuffle mode enabled"
        printf '%s\n' "${files[@]}" | shuf
    else
        log_info "Sequential mode enabled"
        printf '%s\n' "${files[@]}" | sort
    fi
}

# Main streaming loop
stream_playlist() {
    log_info "Starting playlist stream..."
    log_info "Stream URL: $STREAM_URL"
    log_info "Bitrate: ${BITRATE}k (MP3)"

    while true; do
        local file_list
        mapfile -t file_list < <(get_file_list)

        for file in "${file_list[@]}"; do
            stream_file "$file" || continue
        done

        if [ "$LOOP" = false ]; then
            log_info "Playlist finished (loop disabled)"
            break
        fi

        log_info "Playlist finished, restarting..."
        sleep 1
    done

    log_info "Streaming stopped"
}

# Trap for graceful shutdown
trap 'log_info "Interrupted, stopping..."; exit 0' INT TERM

# Main execution
main() {
    log_info "YggRadio Playlist Streamer"
    log_info "=========================="

    check_dependencies
    validate_directory
    stream_playlist
}

main "$@"
`

	t, err := template.New("bash").Parse(tmpl)
	if err != nil {
		return "", fmt.Errorf("failed to parse bash template: %w", err)
	}

	var buf bytes.Buffer
	if err := t.Execute(&buf, config); err != nil {
		return "", fmt.Errorf("failed to execute bash template: %w", err)
	}

	return buf.String(), nil
}

// GeneratePowerShellScript generates a PowerShell script for playlist streaming
func (g *PlaylistScriptGenerator) GeneratePowerShellScript(config PowerShellScriptConfig) (string, error) {
	tmpl := `#
# YggRadio Playlist Streaming Script (PowerShell)
# Generated automatically - DO NOT EDIT
#
# This script streams audio files from a local directory to YggRadio
#

param(
    [switch]$Help
)

# Configuration
$PlaylistDir = "{{.PlaylistDir}}"
$FilePattern = "{{.FilePattern}}"
$Mode = "{{.Mode}}"
$Loop = ${{if .Loop}}true{{else}}false{{end}}
$StreamURL = "{{.StreamURL}}"
$Username = "{{.Username}}"
$Password = "{{.Password}}"
$Bitrate = {{.Bitrate}}

# Logging functions
function Write-Info {
    param([string]$Message)
    Write-Host "[INFO] $Message" -ForegroundColor Green
}

function Write-Warn {
    param([string]$Message)
    Write-Host "[WARN] $Message" -ForegroundColor Yellow
}

function Write-Err {
    param([string]$Message)
    Write-Host "[ERROR] $Message" -ForegroundColor Red
}

# Check dependencies
function Test-Dependencies {
    Write-Info "Checking dependencies..."

    $ffmpeg = Get-Command ffmpeg -ErrorAction SilentlyContinue
    if (-not $ffmpeg) {
        Write-Err "ffmpeg is not installed or not in PATH"
        Write-Info "Download from: https://ffmpeg.org/download.html"
        exit 1
    }

    Write-Info "All dependencies satisfied"
}

# Validate directory
function Test-PlaylistDirectory {
    Write-Info "Validating playlist directory..."

    if (-not (Test-Path -Path $PlaylistDir -PathType Container)) {
        Write-Err "Directory does not exist: $PlaylistDir"
        exit 1
    }

    Write-Info "Directory exists: $PlaylistDir"
}

# Stream a single file
function Start-FileStream {
    param([string]$FilePath)

    $FileName = Split-Path -Leaf $FilePath
    Write-Info "Now playing: $FileName"

    # Update metadata on server
    $songName = [System.IO.Path]::GetFileNameWithoutExtension($FileName)
    try {
        Invoke-WebRequest -Uri "${StreamURL}?mode=updinfo&song=$songName" -Method Get -ErrorAction SilentlyContinue | Out-Null
    }
    catch {
        Write-Warn "Failed to update metadata (continuing anyway)"
    }

    # Brief pause to ensure metadata is updated
    Start-Sleep -Milliseconds 500

    # Stream the file (MP3 only)
    # Create Basic Auth header
    $authBytes = [System.Text.Encoding]::UTF8.GetBytes("${Username}:${Password}")
    $authBase64 = [Convert]::ToBase64String($authBytes)

    try {
        # Create temp file for encoded audio
        $tempFile = "temp_audio_" + [guid]::NewGuid().ToString() + ".mp3"

        # Build ffmpeg command using array
        $ffmpegCmd = @(
            "-re"
            "-i"
            $FilePath
            "-c:a"
            "libmp3lame"
            "-b:a"
            ($Bitrate.ToString() + "k")
            "-ar"
            "44100"
            "-f"
            "mp3"
            "-vn"
            $tempFile
        )

        # Run ffmpeg
        $process = Start-Process -FilePath "ffmpeg" -ArgumentList $ffmpegCmd -NoNewWindow -Wait -PassThru

        if (($process.ExitCode -eq 0) -and (Test-Path $tempFile)) {
            # Read encoded file
            $audioData = [System.IO.File]::ReadAllBytes($tempFile)

            # Create headers
            $headers = @{
                "Authorization" = "Basic " + $authBase64
                "Content-Type" = "audio/mpeg"
            }

            # Upload to server
            Invoke-WebRequest -Uri $StreamURL -Method Put -Headers $headers -Body $audioData | Out-Null

            # Cleanup
            Remove-Item $tempFile -ErrorAction SilentlyContinue
            Write-Info "Finished: $FileName"
            return $true
        }
        else {
            Write-Warn "Failed to encode: $FileName (skipping)"
            if (Test-Path $tempFile) {
                Remove-Item $tempFile -ErrorAction SilentlyContinue
            }
            return $false
        }
    }
    catch {
        Write-Warn "Failed to stream: $FileName - $_"
        return $false
    }

    return $true
}

# Get file list
function Get-PlaylistFiles {
    Write-Info "Scanning for files matching pattern: $FilePattern"

    # Check if pattern contains braces (multiple extensions)
    if ($FilePattern -match '\{.*\}') {
        # Pattern like *.{mp3,ogg,flac} - search for all common audio formats recursively
        $extensions = @("*.mp3", "*.ogg", "*.opus", "*.flac", "*.m4a", "*.aac", "*.wav", "*.wma", "*.ape")
        $files = Get-ChildItem -Path $PlaylistDir -Include $extensions -File -Recurse -ErrorAction SilentlyContinue
    }
    else {
        # Simple pattern like *.mp3 - search recursively
        $files = Get-ChildItem -Path $PlaylistDir -Filter $FilePattern -File -Recurse -ErrorAction SilentlyContinue
    }

    if ($files.Count -eq 0) {
        Write-Err "No files found matching pattern: $FilePattern"
        exit 1
    }

    Write-Info "Found $($files.Count) file(s)"

    if ($Mode -eq "shuffle") {
        Write-Info "Shuffle mode enabled"
        return $files | Get-Random -Count $files.Count
    }
    else {
        Write-Info "Sequential mode enabled"
        return $files | Sort-Object Name
    }
}

# Main streaming loop
function Start-PlaylistStream {
    Write-Info "Starting playlist stream..."
    Write-Info "Stream URL: $StreamURL"
    Write-Info "Bitrate: ${Bitrate}k (MP3)"

    do {
        $fileList = Get-PlaylistFiles

        foreach ($file in $fileList) {
            Start-FileStream -FilePath $file.FullName
        }

        if (-not $Loop) {
            Write-Info "Playlist finished (loop disabled)"
            break
        }

        Write-Info "Playlist finished, restarting..."
        Start-Sleep -Seconds 1
    } while ($true)

    Write-Info "Streaming stopped"
}

# Main execution
function Main {
    Write-Info "YggRadio Playlist Streamer"
    Write-Info "=========================="

    Test-Dependencies
    Test-PlaylistDirectory
    Start-PlaylistStream
}

# Handle Ctrl+C gracefully
$null = Register-EngineEvent -SourceIdentifier PowerShell.Exiting -Action {
    Write-Info "Interrupted, stopping..."
}

Main
`

	t, err := template.New("powershell").Parse(tmpl)
	if err != nil {
		return "", fmt.Errorf("failed to parse powershell template: %w", err)
	}

	var buf bytes.Buffer
	if err := t.Execute(&buf, config); err != nil {
		return "", fmt.Errorf("failed to execute powershell template: %w", err)
	}

	return buf.String(), nil
}

// GenerateSystemdUnit generates a systemd service unit file
func (g *PlaylistScriptGenerator) GenerateSystemdUnit(config SystemdUnitConfig) (string, error) {
	tmpl := `[Unit]
Description={{.Description}}
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
WorkingDirectory={{.WorkingDir}}
ExecStart={{.ScriptPath}}
Restart=on-failure
RestartSec=10
StandardOutput=journal
StandardError=journal

# Security settings
NoNewPrivileges=true
PrivateTmp=true

[Install]
WantedBy=multi-user.target
`

	t, err := template.New("systemd").Parse(tmpl)
	if err != nil {
		return "", fmt.Errorf("failed to parse systemd template: %w", err)
	}

	var buf bytes.Buffer
	if err := t.Execute(&buf, config); err != nil {
		return "", fmt.Errorf("failed to execute systemd template: %w", err)
	}

	return buf.String(), nil
}
