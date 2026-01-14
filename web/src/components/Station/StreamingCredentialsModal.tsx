import { useState, useEffect } from 'react';
import { X, Copy, Check, Radio, Terminal, Download, Settings, ChevronDown, ChevronUp } from 'lucide-react';
import { getStreamingCredentials, StreamingCredentials, getServerInfo, ServerInfo, updatePlaylistConfig, downloadPlaylistScript, PlaylistConfig, fetchCSRFToken } from '../../lib/api';
import { Station, usePlayerStore } from '../../stores/playerStore';

interface StreamingCredentialsModalProps {
  isOpen: boolean;
  onClose: () => void;
  station: Station;
}

export default function StreamingCredentialsModal({
  isOpen,
  onClose,
  station,
}: StreamingCredentialsModalProps) {
  const [credentials, setCredentials] = useState<StreamingCredentials | null>(null);
  const [serverInfo, setServerInfo] = useState<ServerInfo | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [copiedField, setCopiedField] = useState<string | null>(null);
  const [copyError, setCopyError] = useState(false);
  const [successMessage, setSuccessMessage] = useState('');
  const { currentStreamUrl } = usePlayerStore();

  // Playlist configuration state
  const [playlistEnabled, setPlaylistEnabled] = useState(false);
  const [playlistDir, setPlaylistDir] = useState('');
  const [playlistMode, setPlaylistMode] = useState<'sequential' | 'shuffle'>('sequential');
  const [playlistLoop, setPlaylistLoop] = useState(true);
  const [playlistPattern, setPlaylistPattern] = useState('*.{mp3,ogg,opus,flac,m4a,aac,wav,wma,ape}');
  const [saving, setSaving] = useState(false);
  const [playlistSectionExpanded, setPlaylistSectionExpanded] = useState(false);

  useEffect(() => {
    if (isOpen) {
      loadData();
      // Initialize playlist config from station data
      setPlaylistEnabled(station.playlist_enabled || false);
      setPlaylistDir(station.playlist_directory || '');
      setPlaylistMode(station.playlist_mode || 'sequential');
      setPlaylistLoop(station.playlist_loop !== undefined ? station.playlist_loop : true);
      setPlaylistPattern(station.playlist_file_pattern || '*.{mp3,ogg,opus,flac,m4a,aac,wav,wma,ape}');
    }
  }, [isOpen, station.id]);

  const loadData = async () => {
    setLoading(true);
    setError('');
    try {
      const [creds, info] = await Promise.all([
        getStreamingCredentials(station.id),
        getServerInfo(),
      ]);
      setCredentials(creds);
      setServerInfo(info);
    } catch (err: any) {
      setError(err.message || 'Failed to load credentials');
    } finally {
      setLoading(false);
    }
  };

  const copyToClipboard = async (text: string, field: string) => {
    setCopyError(false);
    try {
      // Try modern clipboard API first
      if (navigator.clipboard && navigator.clipboard.writeText) {
        await navigator.clipboard.writeText(text);
        setCopiedField(field);
        setTimeout(() => setCopiedField(null), 2000);
      } else {
        // Fallback to older method
        const textArea = document.createElement('textarea');
        textArea.value = text;
        textArea.style.position = 'fixed';
        textArea.style.left = '-999999px';
        textArea.style.top = '-999999px';
        document.body.appendChild(textArea);
        textArea.focus();
        textArea.select();

        try {
          document.execCommand('copy');
          setCopiedField(field);
          setTimeout(() => setCopiedField(null), 2000);
        } catch (err) {
          console.error('Fallback copy failed:', err);
          setCopyError(true);
          setTimeout(() => setCopyError(false), 3000);
        } finally {
          document.body.removeChild(textArea);
        }
      }
    } catch (err) {
      console.error('Failed to copy:', err);
      setCopyError(true);
      setTimeout(() => setCopyError(false), 3000);
    }
  };

  const handleSavePlaylistConfig = async () => {
    setSaving(true);
    setError('');

    try {
      // Fetch CSRF token
      const csrfToken = await fetchCSRFToken();

      const config: PlaylistConfig = {
        playlist_enabled: playlistEnabled,
        playlist_directory: playlistDir,
        playlist_mode: playlistMode,
        playlist_loop: playlistLoop,
        playlist_file_pattern: playlistPattern,
      };

      await updatePlaylistConfig(station.id, config, csrfToken);

      // Update the station object in parent component if needed
      // For now, just show success
      setSuccessMessage('Playlist configuration saved successfully!');
      setTimeout(() => setSuccessMessage(''), 5000);
    } catch (err: any) {
      setError(err.message || 'Failed to save playlist configuration');
    } finally {
      setSaving(false);
    }
  };

  const handleDownloadScript = async (type: 'bash' | 'powershell' | 'systemd') => {
    try {
      const blob = await downloadPlaylistScript(station.id, type);

      // Create download link
      const url = window.URL.createObjectURL(blob);
      const a = document.createElement('a');
      a.href = url;

      // Set filename based on type
      const filenames = {
        bash: `yggradio-stream.sh`,
        powershell: `yggradio-stream.ps1`,
        systemd: `yggradio-stream.service`,
      };
      a.download = filenames[type];

      // Trigger download
      document.body.appendChild(a);
      a.click();

      // Cleanup
      window.URL.revokeObjectURL(url);
      document.body.removeChild(a);
    } catch (err: any) {
      setError(err.message || `Failed to download ${type} script`);
    }
  };

  if (!isOpen) return null;

  // Check if player is visible (has active stream)
  const isPlayerVisible = !!currentStreamUrl;

  // Get actual server URL from current location and server info
  // Priority: 1. serverInfo.port (from config), 2. window.location.port (from browser URL), 3. default 8080
  const port = serverInfo?.port || parseInt(window.location.port) || 8080;

  // For IPv6 addresses, window.location.hostname already includes brackets
  // Check if hostname already has brackets to avoid double-wrapping
  const hostname = window.location.hostname;
  const hostnameWithBrackets = hostname.includes(':') && !hostname.startsWith('[')
    ? `[${hostname}]`  // IPv6 without brackets - add them
    : hostname;        // IPv4 or IPv6 with brackets already - use as-is

  const serverUrl = `http://${hostnameWithBrackets}:${port}`;

  const ffmpegCommand = credentials
    ? `ffmpeg -re -i your-audio.flac \\
  -codec:a libmp3lame -b:a ${station.bitrate}k \\
  -vn \\
  -f mp3 -content_type audio/mpeg \\
  -method PUT \\
  'http://${credentials.username}:${credentials.password}@${hostnameWithBrackets}:${port}${credentials.mountpoint}'`
    : '';

  return (
    <div className="fixed inset-0 bg-black bg-opacity-50 flex items-start sm:items-center justify-center p-2 sm:p-4 z-50 overflow-y-auto">
      <div
        className={`bg-gray-900 rounded-lg p-4 sm:p-6 max-w-2xl w-full border border-gray-800 my-2 sm:my-4 overflow-y-auto ${
          isPlayerVisible
            ? 'max-h-[calc(100vh-140px)] sm:max-h-[90vh]'
            : 'max-h-[95vh]'
        }`}
        style={isPlayerVisible ? { marginBottom: '100px' } : undefined}
      >
        {/* Header */}
        <div className="flex items-center justify-between mb-4 sm:mb-6">
          <div className="flex items-center gap-2">
            <Radio className="w-5 h-5 sm:w-6 sm:h-6 text-indigo-500" />
            <h2 className="text-xl sm:text-2xl font-bold">Streaming Credentials</h2>
          </div>
          <button
            onClick={onClose}
            className="text-gray-400 hover:text-white transition-colors"
          >
            <X className="w-5 h-5 sm:w-6 sm:h-6" />
          </button>
        </div>

        <div className="mb-4 text-sm sm:text-base text-gray-400">
          Station: <span className="text-white font-semibold">{station.name}</span>
        </div>

        {/* Loading State */}
        {loading && (
          <div className="text-center py-8 text-gray-400">
            Loading credentials...
          </div>
        )}

        {/* Error State */}
        {error && (
          <div className="bg-red-900 bg-opacity-50 border border-red-700 rounded-lg p-4 text-red-200 mb-4">
            {error}
          </div>
        )}

        {/* Success Message */}
        {successMessage && (
          <div className="bg-green-900 bg-opacity-50 border border-green-700 rounded-lg p-4 text-green-200 mb-4">
            {successMessage}
          </div>
        )}

        {/* Copy Error */}
        {copyError && (
          <div className="bg-orange-900 bg-opacity-50 border border-orange-700 rounded-lg p-4 text-orange-200 mb-4">
            Failed to copy to clipboard. Please copy manually.
          </div>
        )}

        {/* External HLS Stream Notice */}
        {station.external_stream_type === 'hls' && (
          <div className="bg-blue-900 bg-opacity-50 border border-blue-700 rounded-lg p-4 text-blue-200 mb-4">
            <p className="font-medium mb-2">External HLS Stream</p>
            <p className="text-sm text-blue-300">
              This station streams from an external HLS source. No source credentials are needed - the stream is automatically relayed to listeners.
            </p>
          </div>
        )}

        {/* Credentials - Only for non-HLS streams */}
        {!loading && !error && credentials && station.external_stream_type !== 'hls' && (
          <div className="space-y-3 sm:space-y-4">
            {/* Username */}
            <div>
              <label className="block text-xs sm:text-sm font-medium mb-1 text-gray-400">
                Username
              </label>
              <div className="flex flex-col sm:flex-row items-stretch sm:items-center gap-2">
                <input
                  type="text"
                  value={credentials.username}
                  readOnly
                  className="flex-1 p-2 bg-gray-800 border border-gray-700 rounded font-mono text-xs sm:text-sm"
                />
                <button
                  onClick={() => copyToClipboard(credentials.username, 'username')}
                  className="p-2 bg-gray-800 hover:bg-gray-700 border border-gray-700 rounded transition-colors"
                  title="Copy username"
                >
                  {copiedField === 'username' ? (
                    <Check className="w-4 h-4 sm:w-5 sm:h-5 text-green-500 mx-auto" />
                  ) : (
                    <Copy className="w-4 h-4 sm:w-5 sm:h-5 mx-auto" />
                  )}
                </button>
              </div>
            </div>

            {/* Password */}
            <div>
              <label className="block text-xs sm:text-sm font-medium mb-1 text-gray-400">
                Password
              </label>
              <div className="flex flex-col sm:flex-row items-stretch sm:items-center gap-2">
                <input
                  type="text"
                  value={credentials.password}
                  readOnly
                  className="flex-1 p-2 bg-gray-800 border border-gray-700 rounded font-mono text-xs sm:text-sm break-all"
                />
                <button
                  onClick={() => copyToClipboard(credentials.password, 'password')}
                  className="p-2 bg-gray-800 hover:bg-gray-700 border border-gray-700 rounded transition-colors"
                  title="Copy password"
                >
                  {copiedField === 'password' ? (
                    <Check className="w-4 h-4 sm:w-5 sm:h-5 text-green-500 mx-auto" />
                  ) : (
                    <Copy className="w-4 h-4 sm:w-5 sm:h-5 mx-auto" />
                  )}
                </button>
              </div>
            </div>

            {/* Mountpoint */}
            <div>
              <label className="block text-xs sm:text-sm font-medium mb-1 text-gray-400">
                Mountpoint
              </label>
              <div className="flex flex-col sm:flex-row items-stretch sm:items-center gap-2">
                <input
                  type="text"
                  value={credentials.mountpoint}
                  readOnly
                  className="flex-1 p-2 bg-gray-800 border border-gray-700 rounded font-mono text-xs sm:text-sm"
                />
                <button
                  onClick={() => copyToClipboard(credentials.mountpoint, 'mountpoint')}
                  className="p-2 bg-gray-800 hover:bg-gray-700 border border-gray-700 rounded transition-colors"
                  title="Copy mountpoint"
                >
                  {copiedField === 'mountpoint' ? (
                    <Check className="w-4 h-4 sm:w-5 sm:h-5 text-green-500 mx-auto" />
                  ) : (
                    <Copy className="w-4 h-4 sm:w-5 sm:h-5 mx-auto" />
                  )}
                </button>
              </div>
            </div>

            {/* Stream URL */}
            <div>
              <label className="block text-xs sm:text-sm font-medium mb-1 text-gray-400">
                Stream URL (for source)
              </label>
              <div className="flex flex-col sm:flex-row items-stretch sm:items-center gap-2">
                <input
                  type="text"
                  value={`${serverUrl}${credentials.mountpoint}`}
                  readOnly
                  className="flex-1 p-2 bg-gray-800 border border-gray-700 rounded font-mono text-xs sm:text-sm break-all"
                />
                <button
                  onClick={() => copyToClipboard(`${serverUrl}${credentials.mountpoint}`, 'url')}
                  className="p-2 bg-gray-800 hover:bg-gray-700 border border-gray-700 rounded transition-colors"
                  title="Copy URL"
                >
                  {copiedField === 'url' ? (
                    <Check className="w-4 h-4 sm:w-5 sm:h-5 text-green-500 mx-auto" />
                  ) : (
                    <Copy className="w-4 h-4 sm:w-5 sm:h-5 mx-auto" />
                  )}
                </button>
              </div>
            </div>

            {/* FFmpeg Command */}
            <div className="mt-4 sm:mt-6">
              <div className="flex items-center gap-2 mb-2">
                <Terminal className="w-4 h-4 sm:w-5 sm:h-5 text-indigo-400" />
                <label className="text-xs sm:text-sm font-medium text-gray-400">
                  FFmpeg Streaming Command
                </label>
              </div>
              <div className="relative">
                <pre className="p-2 sm:p-3 bg-gray-950 border border-gray-700 rounded text-xs overflow-x-auto">
                  <code className="text-green-400">{ffmpegCommand}</code>
                </pre>
                <button
                  onClick={() => copyToClipboard(ffmpegCommand, 'ffmpeg')}
                  className="absolute top-2 right-2 p-1.5 sm:p-2 bg-gray-800 hover:bg-gray-700 border border-gray-700 rounded transition-colors"
                  title="Copy command"
                >
                  {copiedField === 'ffmpeg' ? (
                    <Check className="w-3 h-3 sm:w-4 sm:h-4 text-green-500" />
                  ) : (
                    <Copy className="w-3 h-3 sm:w-4 sm:h-4" />
                  )}
                </button>
              </div>
            </div>

            {/* Automated Playlist Streaming */}
            <div className="mt-4 sm:mt-6 border border-gray-700 rounded overflow-hidden">
              {/* Section Header */}
              <button
                onClick={() => setPlaylistSectionExpanded(!playlistSectionExpanded)}
                className="w-full flex items-center justify-between p-3 sm:p-4 bg-gray-800 hover:bg-gray-750 transition-colors"
              >
                <div className="flex items-center gap-2">
                  <Settings className="w-4 h-4 sm:w-5 sm:h-5 text-purple-400" />
                  <h3 className="text-sm sm:text-base font-semibold text-purple-300">
                    Automated Playlist Streaming
                  </h3>
                </div>
                {playlistSectionExpanded ? (
                  <ChevronUp className="w-4 h-4 sm:w-5 sm:h-5 text-gray-400" />
                ) : (
                  <ChevronDown className="w-4 h-4 sm:w-5 sm:h-5 text-gray-400" />
                )}
              </button>

              {/* Section Content */}
              {playlistSectionExpanded && (
                <div className="p-3 sm:p-4 bg-gray-850 space-y-3 sm:space-y-4">
                  {/* Enable Checkbox */}
                  <div className="flex items-center gap-2">
                    <input
                      type="checkbox"
                      id="playlist-enabled"
                      checked={playlistEnabled}
                      onChange={(e) => setPlaylistEnabled(e.target.checked)}
                      className="w-4 h-4 rounded bg-gray-700 border-gray-600 text-purple-500 focus:ring-purple-500 focus:ring-offset-gray-900"
                    />
                    <label htmlFor="playlist-enabled" className="text-sm text-gray-300">
                      Enable automated playlist streaming
                    </label>
                  </div>

                  {/* Configuration Fields (only visible when enabled) */}
                  {playlistEnabled && (
                    <>
                      {/* Directory Path */}
                      <div>
                        <label className="block text-xs sm:text-sm font-medium mb-1 text-gray-400">
                          Local Directory Path
                        </label>
                        <input
                          type="text"
                          value={playlistDir}
                          onChange={(e) => setPlaylistDir(e.target.value)}
                          placeholder="/path/to/your/music"
                          className="w-full p-2 bg-gray-800 border border-gray-700 rounded text-sm focus:ring-2 focus:ring-purple-500 focus:border-transparent"
                        />
                        <p className="mt-1 text-xs text-gray-500">
                          Path on YOUR local machine where audio files are stored
                        </p>
                      </div>

                      {/* File Pattern */}
                      <div>
                        <label className="block text-xs sm:text-sm font-medium mb-1 text-gray-400">
                          File Pattern
                        </label>
                        <input
                          type="text"
                          value={playlistPattern}
                          onChange={(e) => setPlaylistPattern(e.target.value)}
                          placeholder="*.{mp3,ogg,opus,flac,m4a,aac,wav,wma,ape}"
                          className="w-full p-2 bg-gray-800 border border-gray-700 rounded text-sm font-mono focus:ring-2 focus:ring-purple-500 focus:border-transparent"
                        />
                        <p className="mt-1 text-xs text-gray-500">
                          Default searches all audio formats (MP3, OGG, Opus, FLAC, M4A, AAC, WAV, WMA, APE).
                          Custom: <code className="text-purple-400">*.mp3</code> or <code className="text-purple-400">*.{"{mp3,flac}"}</code>
                        </p>
                      </div>

                      {/* Mode Selection */}
                      <div>
                        <label className="block text-xs sm:text-sm font-medium mb-2 text-gray-400">
                          Playback Mode
                        </label>
                        <div className="flex gap-4">
                          <div className="flex items-center">
                            <input
                              type="radio"
                              id="mode-sequential"
                              name="playlist-mode"
                              value="sequential"
                              checked={playlistMode === 'sequential'}
                              onChange={(e) => setPlaylistMode(e.target.value as 'sequential' | 'shuffle')}
                              className="w-4 h-4 bg-gray-700 border-gray-600 text-purple-500 focus:ring-purple-500 focus:ring-offset-gray-900"
                            />
                            <label htmlFor="mode-sequential" className="ml-2 text-sm text-gray-300">
                              Sequential
                            </label>
                          </div>
                          <div className="flex items-center">
                            <input
                              type="radio"
                              id="mode-shuffle"
                              name="playlist-mode"
                              value="shuffle"
                              checked={playlistMode === 'shuffle'}
                              onChange={(e) => setPlaylistMode(e.target.value as 'sequential' | 'shuffle')}
                              className="w-4 h-4 bg-gray-700 border-gray-600 text-purple-500 focus:ring-purple-500 focus:ring-offset-gray-900"
                            />
                            <label htmlFor="mode-shuffle" className="ml-2 text-sm text-gray-300">
                              Shuffle
                            </label>
                          </div>
                        </div>
                      </div>

                      {/* Loop Checkbox */}
                      <div className="flex items-center gap-2">
                        <input
                          type="checkbox"
                          id="playlist-loop"
                          checked={playlistLoop}
                          onChange={(e) => setPlaylistLoop(e.target.checked)}
                          className="w-4 h-4 rounded bg-gray-700 border-gray-600 text-purple-500 focus:ring-purple-500 focus:ring-offset-gray-900"
                        />
                        <label htmlFor="playlist-loop" className="text-sm text-gray-300">
                          Loop playlist (restart when finished)
                        </label>
                      </div>

                      {/* Save Button */}
                      <div className="pt-2">
                        <button
                          onClick={handleSavePlaylistConfig}
                          disabled={saving || !playlistDir}
                          className="w-full px-4 py-2 bg-purple-600 hover:bg-purple-700 disabled:bg-gray-700 disabled:text-gray-500 rounded transition-colors text-sm sm:text-base font-medium"
                        >
                          {saving ? 'Saving...' : 'Save Playlist Configuration'}
                        </button>
                      </div>

                      {/* Download Scripts Section */}
                      <div className="pt-4 border-t border-gray-700">
                        <h4 className="text-sm font-semibold mb-3 text-gray-300">Download Streaming Scripts</h4>
                        <div className="grid grid-cols-1 sm:grid-cols-3 gap-2">
                          <button
                            onClick={() => handleDownloadScript('bash')}
                            className="flex items-center justify-center gap-2 px-3 py-2 bg-gray-700 hover:bg-gray-600 rounded transition-colors text-sm"
                          >
                            <Download className="w-4 h-4" />
                            <span>Bash Script</span>
                          </button>
                          <button
                            onClick={() => handleDownloadScript('powershell')}
                            className="flex items-center justify-center gap-2 px-3 py-2 bg-gray-700 hover:bg-gray-600 rounded transition-colors text-sm"
                          >
                            <Download className="w-4 h-4" />
                            <span>PowerShell</span>
                          </button>
                          <button
                            onClick={() => handleDownloadScript('systemd')}
                            className="flex items-center justify-center gap-2 px-3 py-2 bg-gray-700 hover:bg-gray-600 rounded transition-colors text-sm"
                          >
                            <Download className="w-4 h-4" />
                            <span>Systemd Unit</span>
                          </button>
                        </div>
                        <p className="mt-2 text-xs text-gray-500">
                          Linux/macOS: Bash | Windows: PowerShell | Linux background: Systemd
                        </p>
                      </div>

                      {/* Usage Instructions */}
                      <div className="pt-4 border-t border-gray-700">
                        <h4 className="text-sm font-semibold mb-2 text-gray-300">How to Use:</h4>
                        <div className="space-y-3 text-xs text-gray-400">
                          {/* Bash Instructions */}
                          <div>
                            <p className="font-semibold text-green-400 mb-1">Linux/macOS (One-time):</p>
                            <pre className="p-2 bg-gray-950 border border-gray-700 rounded overflow-x-auto">
                              <code className="text-green-300">chmod +x yggradio-stream.sh{'\n'}./yggradio-stream.sh</code>
                            </pre>
                          </div>

                          {/* Systemd Instructions */}
                          <div>
                            <p className="font-semibold text-blue-400 mb-1">Linux (Background Service with Systemd):</p>
                            <pre className="p-2 bg-gray-950 border border-gray-700 rounded overflow-x-auto">
                              <code className="text-blue-300"># 1. Download bash script first, then systemd unit{'\n'}chmod +x yggradio-stream.sh{'\n'}sudo cp yggradio-stream.sh /usr/local/bin/{'\n'}{'\n'}# 2. Install and start service{'\n'}sudo cp yggradio-*.service /etc/systemd/system/{'\n'}sudo systemctl daemon-reload{'\n'}sudo systemctl enable --now yggradio-*.service{'\n'}{'\n'}# Check status{'\n'}sudo systemctl status yggradio-*</code>
                            </pre>
                          </div>

                          {/* Screen/Tmux Instructions */}
                          <div>
                            <p className="font-semibold text-yellow-400 mb-1">Screen/Tmux (Detached Session):</p>
                            <pre className="p-2 bg-gray-950 border border-gray-700 rounded overflow-x-auto">
                              <code className="text-yellow-300">screen -S yggradio ./yggradio-stream.sh{'\n'}# Detach: Ctrl+A, then D</code>
                            </pre>
                          </div>

                          {/* PowerShell Instructions */}
                          <div>
                            <p className="font-semibold text-purple-400 mb-1">Windows PowerShell:</p>
                            <pre className="p-2 bg-gray-950 border border-gray-700 rounded overflow-x-auto">
                              <code className="text-purple-300">powershell -ExecutionPolicy Bypass -File yggradio-stream.ps1</code>
                            </pre>
                          </div>
                        </div>
                      </div>
                    </>
                  )}
                </div>
              )}
            </div>

            {/* Instructions */}
            <div className="mt-4 sm:mt-6 p-3 sm:p-4 bg-indigo-900 bg-opacity-30 border border-indigo-700 rounded">
              <h3 className="text-sm sm:text-base font-semibold mb-2 text-indigo-300">How to Stream:</h3>
              <ol className="list-decimal list-inside space-y-1 text-xs sm:text-sm text-gray-300">
                <li>Replace <code className="text-indigo-400">your-audio.flac</code> with your audio file</li>
                <li>Run the ffmpeg command in your terminal</li>
                <li>Your station will go online automatically</li>
                <li>Your listeners can play the stream in the web browser</li>
              </ol>
              <p className="mt-3 text-xs text-gray-400">
                <strong>Note:</strong> Always use MP3 codec (libmp3lame) with <code>-vn</code> flag to exclude video/images
              </p>
            </div>
          </div>
        )}

        {/* Close Button */}
        <div className="mt-4 sm:mt-6 flex justify-end">
          <button
            onClick={onClose}
            className="px-4 sm:px-6 py-2 bg-gray-800 hover:bg-gray-700 rounded transition-colors text-sm sm:text-base"
          >
            Close
          </button>
        </div>
      </div>
    </div>
  );
}
