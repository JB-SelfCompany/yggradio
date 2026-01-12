import { useState } from 'react';
import { X, Radio, AlertCircle } from 'lucide-react';
import { api, fetchCSRFToken, getPoWDifficulty } from '../../lib/api';
import { useAuthStore } from '../../stores/authStore';
import { calculatePoW, base64ToHex } from '../../lib/pow';

interface CreateStationModalProps {
  isOpen: boolean;
  onClose: () => void;
  onSuccess: () => void;
}

export default function CreateStationModal({
  isOpen,
  onClose,
  onSuccess,
}: CreateStationModalProps) {
  const [name, setName] = useState('');
  const [description, setDescription] = useState('');
  const [mountpoint, setMountpoint] = useState('');
  const [contentType, setContentType] = useState('audio/mpeg');
  const [bitrate, setBitrate] = useState(96);
  const [isPrivate, setIsPrivate] = useState(false);
  const [streamMode, setStreamMode] = useState<'direct' | 'external'>('direct');
  const [externalStreamUrl, setExternalStreamUrl] = useState('');
  const [externalStreamType, setExternalStreamType] = useState<'hls' | 'direct'>('hls');
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');
  const { publicKey, isAuthenticated, authMethod, userId } = useAuthStore();

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setError('');

    if (!isAuthenticated) {
      setError('You must be logged in to create a station');
      return;
    }

    // Validate mountpoint
    const cleanMountpoint = mountpoint.trim().startsWith('/')
      ? mountpoint.trim()
      : `/${mountpoint.trim()}`;

    if (!/^\/[a-zA-Z0-9_-]+$/.test(cleanMountpoint)) {
      setError('Mountpoint must start with / and contain only letters, numbers, dashes, and underscores');
      return;
    }

    // Validate external stream URL if in external mode
    if (streamMode === 'external') {
      const urlTrimmed = externalStreamUrl.trim();
      if (!urlTrimmed) {
        setError('External stream URL is required for external mode');
        return;
      }
      // Basic URL validation
      try {
        const url = new URL(urlTrimmed);
        if (url.protocol !== 'http:' && url.protocol !== 'https:') {
          setError('External stream URL must use HTTP or HTTPS protocol');
          return;
        }
      } catch {
        setError('Invalid external stream URL format');
        return;
      }
    }

    setLoading(true);

    try {
      // Debug: log auth state
      console.log('CreateStation: Auth state:', { isAuthenticated, authMethod, userId, hasPublicKey: !!publicKey });

      // Get PoW difficulty from server config
      const difficulty = await getPoWDifficulty('station');

      // Calculate Proof-of-Work
      // For Ed25519: use actual pubkey (convert from base64 to hex)
      // For Magic Link: generate synthetic pubkey (same format as backend)
      const timestamp = Math.floor(Date.now() / 1000);
      let pubkeyForPoW: string;

      if (authMethod === 'ed25519' && publicKey) {
        // Ed25519 users: convert base64 pubkey to hex
        pubkeyForPoW = base64ToHex(publicKey);
        console.log('CreateStation: Using Ed25519 pubkey');
      } else if (authMethod === 'magic_link' && userId) {
        // Magic Link users: generate synthetic pubkey (64 hex chars)
        // Format: "magiclink:" (10 chars) + padded user_id (54 chars) = 64 chars
        pubkeyForPoW = `magiclink:${userId.toString().padStart(54, '0')}`;
        console.log('CreateStation: Generated synthetic pubkey:', pubkeyForPoW);
      } else {
        setError(`Authentication error: authMethod=${authMethod}, userId=${userId}, hasPublicKey=${!!publicKey}`);
        return;
      }

      const nonce = await calculatePoW(pubkeyForPoW, timestamp, difficulty);

      // Get CSRF token
      const csrfToken = await fetchCSRFToken();

      // Prepare request body
      const requestBody: any = {
        name,
        description,
        mountpoint: cleanMountpoint,
        content_type: contentType,
        bitrate,
        is_private: isPrivate,
        pow_nonce: nonce,
        pow_timestamp: timestamp,
      };

      // Add external stream fields if in external mode
      if (streamMode === 'external') {
        requestBody.external_stream_url = externalStreamUrl.trim();
        requestBody.external_stream_type = externalStreamType;
      }

      // Create station with authentication
      await api.post(
        '/stations',
        requestBody,
        true, // requiresAuth
        csrfToken
      );

      // Success - clear form and close
      setName('');
      setDescription('');
      setMountpoint('');
      setContentType('audio/mpeg');
      setBitrate(96);
      setIsPrivate(false);
      setStreamMode('direct');
      setExternalStreamUrl('');
      setExternalStreamType('hls');
      onSuccess();
      onClose();
    } catch (err: any) {
      setError(err.message || 'Failed to create station');
    } finally {
      setLoading(false);
    }
  };

  if (!isOpen) return null;

  return (
    <div className="fixed inset-0 bg-black bg-opacity-50 flex items-start justify-center p-4 z-50 overflow-y-auto">
      <div className="bg-gray-900 rounded-lg p-6 max-w-md w-full border border-gray-800 my-8">
        {/* Header */}
        <div className="flex items-center justify-between mb-6">
          <div className="flex items-center gap-2">
            <Radio className="w-6 h-6 text-indigo-500" />
            <h2 className="text-2xl font-bold">Create Radio Station</h2>
          </div>
          <button
            onClick={onClose}
            className="text-gray-400 hover:text-white transition-colors"
            disabled={loading}
          >
            <X className="w-6 h-6" />
          </button>
        </div>

        {/* Error Alert */}
        {error && (
          <div className="mb-4 p-3 bg-red-900 bg-opacity-50 border border-red-700 rounded-lg flex items-start gap-2">
            <AlertCircle className="w-5 h-5 text-red-400 flex-shrink-0 mt-0.5" />
            <p className="text-sm text-red-200">{error}</p>
          </div>
        )}

        {/* Form */}
        <form onSubmit={handleSubmit} className="space-y-4">
          {/* Station Name */}
          <div>
            <label className="block text-sm font-medium mb-1">
              Station Name <span className="text-red-500">*</span>
            </label>
            <input
              type="text"
              value={name}
              onChange={(e) => setName(e.target.value)}
              placeholder="My Awesome Radio"
              className="w-full p-2 bg-gray-800 border border-gray-700 rounded focus:border-indigo-500 focus:outline-none"
              required
              minLength={3}
              maxLength={100}
              disabled={loading}
            />
            <p className="text-xs text-gray-500 mt-1">3-100 characters</p>
          </div>

          {/* Description */}
          <div>
            <label className="block text-sm font-medium mb-1">
              Description
            </label>
            <textarea
              value={description}
              onChange={(e) => setDescription(e.target.value)}
              placeholder="Broadcasting the best music 24/7..."
              className="w-full p-2 bg-gray-800 border border-gray-700 rounded focus:border-indigo-500 focus:outline-none resize-none"
              rows={3}
              maxLength={500}
              disabled={loading}
            />
            <p className="text-xs text-gray-500 mt-1">Optional, max 500 characters</p>
          </div>

          {/* Mountpoint */}
          <div>
            <label className="block text-sm font-medium mb-1">
              Mountpoint <span className="text-red-500">*</span>
            </label>
            <div className="flex items-center">
              <span className="text-gray-400 mr-1">/</span>
              <input
                type="text"
                value={mountpoint.replace(/^\//, '')}
                onChange={(e) => setMountpoint(e.target.value.replace(/^\//, ''))}
                placeholder="myradio"
                className="flex-1 p-2 bg-gray-800 border border-gray-700 rounded focus:border-indigo-500 focus:outline-none"
                required
                pattern="[a-zA-Z0-9_\-]+"
                disabled={loading}
              />
            </div>
            <p className="text-xs text-gray-500 mt-1">
              URL path for your station (e.g., "myradio" becomes /myradio)
            </p>
          </div>

          {/* Stream Source Mode */}
          <div>
            <label className="block text-sm font-medium mb-2">
              Stream Source <span className="text-red-500">*</span>
            </label>
            <div className="grid grid-cols-2 gap-2">
              <button
                type="button"
                onClick={() => setStreamMode('direct')}
                className={`p-3 rounded border transition-all ${
                  streamMode === 'direct'
                    ? 'bg-indigo-600 border-indigo-500 text-white'
                    : 'bg-gray-800 border-gray-700 hover:border-gray-600'
                }`}
                disabled={loading}
              >
                <div className="font-medium">Direct Streaming</div>
                <div className="text-xs mt-1 opacity-80">OBS, ffmpeg, BUTT</div>
              </button>
              <button
                type="button"
                onClick={() => setStreamMode('external')}
                className={`p-3 rounded border transition-all ${
                  streamMode === 'external'
                    ? 'bg-indigo-600 border-indigo-500 text-white'
                    : 'bg-gray-800 border-gray-700 hover:border-gray-600'
                }`}
                disabled={loading}
              >
                <div className="font-medium">External Source</div>
                <div className="text-xs mt-1 opacity-80">HLS, Icecast URL</div>
              </button>
            </div>
          </div>

          {/* External Stream URL (only show if external mode) */}
          {streamMode === 'external' && (
            <>
              <div>
                <label className="block text-sm font-medium mb-1">
                  External Stream URL <span className="text-red-500">*</span>
                </label>
                <input
                  type="url"
                  value={externalStreamUrl}
                  onChange={(e) => setExternalStreamUrl(e.target.value)}
                  placeholder="https://example.com/stream.m3u8"
                  className="w-full p-2 bg-gray-800 border border-gray-700 rounded focus:border-indigo-500 focus:outline-none"
                  required
                  disabled={loading}
                />
                <p className="text-xs text-gray-500 mt-1">
                  URL of HLS playlist (.m3u8) or direct audio stream
                </p>
              </div>

              <div>
                <label className="block text-sm font-medium mb-1">
                  Stream Type <span className="text-red-500">*</span>
                </label>
                <select
                  value={externalStreamType}
                  onChange={(e) => setExternalStreamType(e.target.value as 'hls' | 'direct')}
                  className="w-full p-2 bg-gray-800 border border-gray-700 rounded focus:border-indigo-500 focus:outline-none"
                  disabled={loading}
                >
                  <option value="hls">HLS Playlist (.m3u8)</option>
                  <option value="direct">Direct Audio Stream (MP3/AAC/Ogg)</option>
                </select>
                <p className="text-xs text-gray-500 mt-1">
                  Select the type of external stream
                </p>
              </div>
            </>
          )}

          {/* Bitrate (only for direct streaming) */}
          {streamMode === 'direct' && (
            <div>
              <label className="block text-sm font-medium mb-1">
                Bitrate (kbps) <span className="text-red-500">*</span>
              </label>
              <select
                value={bitrate}
                onChange={(e) => setBitrate(parseInt(e.target.value))}
                className="w-full p-2 bg-gray-800 border border-gray-700 rounded focus:border-indigo-500 focus:outline-none"
                disabled={loading}
              >
                <option value="64">64 kbps (Low quality)</option>
                <option value="96">96 kbps (Recommended for Yggdrasil)</option>
                <option value="128">128 kbps (Good quality)</option>
                <option value="192">192 kbps (High quality)</option>
                <option value="256">256 kbps (Very high quality)</option>
                <option value="320">320 kbps (Maximum quality)</option>
              </select>
              <p className="text-xs text-gray-500 mt-1">
                MP3 codec (libmp3lame) - choose bitrate based on your quality needs
              </p>
            </div>
          )}

          {/* Privacy */}
          <div>
            <label className="flex items-center gap-2 cursor-pointer">
              <input
                type="checkbox"
                checked={isPrivate}
                onChange={(e) => setIsPrivate(e.target.checked)}
                className="w-4 h-4 bg-gray-800 border border-gray-700 rounded focus:ring-2 focus:ring-indigo-500"
                disabled={loading}
              />
              <span className="text-sm font-medium">Private Station</span>
            </label>
            <p className="text-xs text-gray-500 mt-1">
              Private stations are only visible to you and won't appear in public listings
            </p>
          </div>

          {/* Info Box */}
          <div className="p-3 bg-indigo-900 bg-opacity-30 border border-indigo-700 rounded text-sm">
            <p className="font-medium mb-1">After creating your station:</p>
            {streamMode === 'direct' ? (
              <ol className="list-decimal list-inside space-y-1 text-gray-300">
                <li>Copy the mountpoint URL</li>
                <li>Start streaming with OBS, ffmpeg, or BUTT</li>
                <li>Your station will appear online automatically</li>
              </ol>
            ) : (
              <ol className="list-decimal list-inside space-y-1 text-gray-300">
                <li>YggRadio will automatically connect to the external stream</li>
                <li>The stream will be re-broadcast to listeners</li>
                <li>Monitor the station status for connection health</li>
              </ol>
            )}
          </div>

          {/* Buttons */}
          <div className="flex gap-3 pt-2">
            <button
              type="button"
              onClick={onClose}
              className="flex-1 py-2 px-4 bg-gray-800 hover:bg-gray-700 rounded transition-colors disabled:opacity-50"
              disabled={loading}
            >
              Cancel
            </button>
            <button
              type="submit"
              className="flex-1 py-2 px-4 bg-indigo-600 hover:bg-indigo-700 rounded transition-colors disabled:opacity-50 flex items-center justify-center gap-2"
              disabled={loading}
            >
              {loading ? (
                <>
                  <div className="w-4 h-4 border-2 border-white border-t-transparent rounded-full animate-spin"></div>
                  Creating...
                </>
              ) : (
                'Create Station'
              )}
            </button>
          </div>
        </form>
      </div>
    </div>
  );
}
