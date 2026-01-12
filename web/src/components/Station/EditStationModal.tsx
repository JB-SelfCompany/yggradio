import { useState, useEffect } from 'react';
import { X, Radio, AlertCircle } from 'lucide-react';
import { api, fetchCSRFToken } from '../../lib/api';
import { Station, usePlayerStore } from '../../stores/playerStore';

interface EditStationModalProps {
  isOpen: boolean;
  onClose: () => void;
  onSuccess: () => void;
  station: Station;
}

export default function EditStationModal({
  isOpen,
  onClose,
  onSuccess,
  station,
}: EditStationModalProps) {
  const [name, setName] = useState('');
  const [description, setDescription] = useState('');
  const [isPrivate, setIsPrivate] = useState(false);
  const [streamMode, setStreamMode] = useState<'direct' | 'external'>('direct');
  const [externalStreamUrl, setExternalStreamUrl] = useState('');
  const [externalStreamType, setExternalStreamType] = useState<'hls' | 'direct'>('hls');
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');
  const { currentStreamUrl } = usePlayerStore();

  // Initialize form with station data
  useEffect(() => {
    if (isOpen && station) {
      setName(station.name);
      setDescription(station.description || '');
      setIsPrivate(station.is_private || false);

      // Initialize external stream fields
      if (station.external_stream_url) {
        setStreamMode('external');
        setExternalStreamUrl(station.external_stream_url);
        setExternalStreamType(station.external_stream_type || 'hls');
      } else {
        setStreamMode('direct');
        setExternalStreamUrl('');
        setExternalStreamType('hls');
      }

      setError('');
    }
  }, [isOpen, station]);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setError('');

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
      // Get CSRF token
      const csrfToken = await fetchCSRFToken();

      // Prepare request body
      const requestBody: any = {
        name,
        description,
        is_private: isPrivate,
      };

      // Add external stream fields based on mode
      if (streamMode === 'external') {
        requestBody.external_stream_url = externalStreamUrl.trim();
        requestBody.external_stream_type = externalStreamType;
      } else {
        // Clear external fields if switching to direct mode
        requestBody.external_stream_url = null;
        requestBody.external_stream_type = null;
      }

      // Update station with authentication
      await api.put(
        `/stations/${station.id}`,
        requestBody,
        true, // requiresAuth
        csrfToken
      );

      // Success
      onSuccess();
      onClose();
    } catch (err: any) {
      setError(err.message || 'Failed to update station');
    } finally {
      setLoading(false);
    }
  };

  if (!isOpen) return null;

  // Check if player is visible (has active stream)
  const isPlayerVisible = !!currentStreamUrl;

  return (
    <div className="fixed inset-0 bg-black bg-opacity-50 flex items-start sm:items-center justify-center p-2 sm:p-4 z-50 overflow-y-auto">
      <div
        className={`bg-gray-900 rounded-lg p-4 sm:p-6 max-w-md w-full border border-gray-800 my-2 sm:my-4 ${
          isPlayerVisible
            ? 'max-h-[calc(100vh-140px)] sm:max-h-[90vh] overflow-y-auto'
            : 'max-h-[95vh] overflow-y-auto'
        }`}
        style={isPlayerVisible ? { marginBottom: '100px' } : undefined}
      >
        {/* Header */}
        <div className="flex items-center justify-between mb-6">
          <div className="flex items-center gap-2">
            <Radio className="w-6 h-6 text-indigo-500" />
            <h2 className="text-2xl font-bold">Edit Station</h2>
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

          {/* Mountpoint (Read-only) */}
          <div>
            <label className="block text-sm font-medium mb-1">
              Mountpoint
            </label>
            <input
              type="text"
              value={station.mountpoint}
              readOnly
              className="w-full p-2 bg-gray-800 border border-gray-700 rounded text-gray-500 cursor-not-allowed"
            />
            <p className="text-xs text-gray-500 mt-1">
              Mountpoint cannot be changed after creation
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
                  Saving...
                </>
              ) : (
                'Save Changes'
              )}
            </button>
          </div>
        </form>
      </div>
    </div>
  );
}
