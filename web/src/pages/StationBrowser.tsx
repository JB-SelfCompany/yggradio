import { useQuery } from '@tanstack/react-query';
import { useState } from 'react';
import { Radio, Users, Play, Search, Plus, Key, Edit, Trash2, StopCircle } from 'lucide-react';
import { api, base64ToHex, fetchCSRFToken } from '../lib/api';
import { usePlayerStore, Station } from '../stores/playerStore';
import { useAuthStore } from '../stores/authStore';
import { stripHTML } from '../lib/sanitize';
import CreateStationModal from '../components/Station/CreateStationModal';
import StreamingCredentialsModal from '../components/Station/StreamingCredentialsModal';
import EditStationModal from '../components/Station/EditStationModal';
import RatingComponent from '../components/Station/RatingComponent';

export default function StationBrowser() {
  const [searchQuery, setSearchQuery] = useState('');
  const [isCreateModalOpen, setIsCreateModalOpen] = useState(false);
  const [credentialsStation, setCredentialsStation] = useState<Station | null>(null);
  const [editStation, setEditStation] = useState<Station | null>(null);
  const [deleteConfirmStation, setDeleteConfirmStation] = useState<Station | null>(null);
  const { playStation, currentStation, isPlaying } = usePlayerStore();
  const { isAuthenticated, publicKey } = useAuthStore();

  // Fetch stations with optional auth to see private stations if logged in
  const { data: stationsResponse, isLoading, error, refetch } = useQuery<{ stations: Station[] }>({
    queryKey: ['stations', isAuthenticated], // Re-fetch when auth state changes
    queryFn: () => api.get<{ stations: Station[] }>('/stations', false, true), // optionalAuth = true
    refetchInterval: 10000, // Refresh every 10 seconds
  });

  const stations = stationsResponse?.stations;

  const handleStationCreated = () => {
    refetch(); // Refresh station list after creation
  };

  const filteredStations = stations?.filter((station) =>
    station.name.toLowerCase().includes(searchQuery.toLowerCase()) ||
    station.description?.toLowerCase().includes(searchQuery.toLowerCase())
  );

  const handlePlayStation = (station: Station) => {
    if (station.status === 'offline') {
      alert('This station is currently offline');
      return;
    }
    playStation(station);
  };

  const isCurrentStation = (station: Station) => {
    return currentStation?.uuid === station.uuid;
  };

  const isOwner = (station: Station) => {
    if (!isAuthenticated || !publicKey) return false;
    const pubkeyHex = base64ToHex(publicKey);
    return station.owner_pubkey === pubkeyHex;
  };

  const showCredentials = (station: Station) => {
    setCredentialsStation(station);
  };

  const handleStopBroadcast = async (station: Station) => {
    if (!confirm(`Stop broadcasting "${station.name}"? This will disconnect all listeners.`)) {
      return;
    }

    try {
      const csrfToken = await fetchCSRFToken();
      await api.post(`/stations/${station.id}/stop`, {}, true, csrfToken);
      refetch();
    } catch (err: any) {
      alert(err.message || 'Failed to stop broadcast');
    }
  };

  const handleDelete = async (station: Station) => {
    if (!deleteConfirmStation) {
      setDeleteConfirmStation(station);
      return;
    }

    try {
      const csrfToken = await fetchCSRFToken();
      await api.delete(`/stations/${station.id}`, true, csrfToken);
      setDeleteConfirmStation(null);
      refetch();
    } catch (err: any) {
      alert(err.message || 'Failed to delete station');
    }
  };

  return (
    <div className="container mx-auto px-4 py-8">
      {/* Header */}
      <div className="mb-8">
        <div className="flex items-center justify-between mb-2">
          <h1 className="text-3xl font-bold flex items-center gap-2">
            <Radio className="w-8 h-8 text-indigo-500" />
            Live Radio Stations
          </h1>
          {isAuthenticated && (
            <button
              onClick={() => setIsCreateModalOpen(true)}
              className="flex items-center gap-2 px-4 py-2 bg-indigo-600 hover:bg-indigo-700 rounded-lg transition-colors"
            >
              <Plus className="w-5 h-5" />
              Create Station
            </button>
          )}
        </div>
        <p className="text-gray-400">
          Browse and listen to live radio streams in the Yggdrasil network
        </p>
      </div>

      {/* Search */}
      <div className="mb-6">
        <div className="relative">
          <Search className="absolute left-3 top-1/2 transform -translate-y-1/2 w-5 h-5 text-gray-400" />
          <input
            type="text"
            value={searchQuery}
            onChange={(e) => setSearchQuery(e.target.value)}
            placeholder="Search stations..."
            className="w-full pl-10 pr-4 py-3 bg-gray-900 border border-gray-800 rounded-lg focus:border-indigo-500 focus:outline-none"
          />
        </div>
      </div>

      {/* Loading State */}
      {isLoading && (
        <div className="text-center py-12 text-gray-400">
          Loading stations...
        </div>
      )}

      {/* Error State */}
      {error && (
        <div className="bg-red-900 bg-opacity-50 border border-red-700 rounded-lg p-4 text-red-200">
          Failed to load stations. Please try again later.
        </div>
      )}

      {/* Empty State */}
      {!isLoading && !error && filteredStations?.length === 0 && (
        <div className="text-center py-12 text-gray-400">
          {searchQuery ? 'No stations found matching your search' : 'No stations available'}
        </div>
      )}

      {/* Station List */}
      <div className="grid md:grid-cols-2 lg:grid-cols-3 gap-6">
        {filteredStations?.map((station) => (
          <div
            key={station.uuid}
            className={`bg-gray-900 border rounded-lg p-6 transition-all ${
              isCurrentStation(station)
                ? 'border-indigo-500 ring-2 ring-indigo-500 ring-opacity-50'
                : 'border-gray-800 hover:border-gray-700'
            }`}
          >
            {/* Station Header */}
            <div className="flex items-start justify-between mb-4">
              <div className="flex-1 min-w-0">
                <h3 className="text-lg font-semibold truncate mb-1">
                  {station.name}
                </h3>
                <div className="flex items-center gap-2 text-sm text-gray-400">
                  <span className={`w-2 h-2 rounded-full ${
                    station.status === 'online' ? 'bg-green-500' : 'bg-gray-500'
                  }`} />
                  <span>{station.status}</span>
                </div>
              </div>

              <button
                onClick={() => handlePlayStation(station)}
                disabled={station.status === 'offline'}
                className={`p-3 rounded-full transition-colors ${
                  isCurrentStation(station) && isPlaying
                    ? 'bg-indigo-600 text-white'
                    : 'bg-gray-800 hover:bg-gray-700 disabled:opacity-50 disabled:cursor-not-allowed'
                }`}
                aria-label="Play station"
              >
                <Play className="w-5 h-5" />
              </button>
            </div>

            {/* Description */}
            {station.description && (
              <p className="text-sm text-gray-400 mb-4 line-clamp-2">
                {stripHTML(station.description)}
              </p>
            )}

            {/* Current Track */}
            {station.metadata_title && (
              <div className="mb-4 p-3 bg-gray-800 rounded text-sm">
                <div className="text-gray-500 text-xs mb-1">
                  {station.status === 'online' ? 'Now Playing:' : 'Last Playing:'}
                </div>
                <div className="font-medium break-words">{station.metadata_title}</div>
              </div>
            )}

            {/* Ratings */}
            <div className="mb-4">
              <RatingComponent
                stationId={station.id}
                stationUUID={station.uuid}
                isFederated={station.is_federated ?? false}
                isAuthenticated={isAuthenticated}
                ownerPubkey={station.owner_pubkey}
                initialAverageRating={station.average_rating ?? 0}
                initialVoteCount={station.vote_count ?? 0}
                onRatingSubmitted={() => refetch()}
              />
            </div>

            {/* Stats */}
            <div className="flex items-center justify-between text-sm">
              <div className="flex items-center gap-1 text-gray-400">
                <Users className="w-4 h-4" />
                <span>{station.listeners_count || 0} listeners</span>
              </div>
              {station.bitrate && (
                <div className="text-gray-500">
                  {station.bitrate}kbps
                </div>
              )}
            </div>

            {/* Actions */}
            <div className="mt-4 pt-4 border-t border-gray-800 space-y-2">
              {/* Owner-only actions */}
              {isOwner(station) && (
                <>
                  <button
                    onClick={() => showCredentials(station)}
                    className="w-full flex items-center justify-center gap-2 px-3 py-2 bg-indigo-600 hover:bg-indigo-700 rounded text-sm transition-colors"
                  >
                    <Key className="w-4 h-4" />
                    Stream Key
                  </button>

                  <div className="flex gap-2">
                    <button
                      onClick={() => setEditStation(station)}
                      className="flex-1 flex items-center justify-center gap-2 px-3 py-2 bg-gray-800 hover:bg-gray-700 rounded text-sm transition-colors"
                    >
                      <Edit className="w-4 h-4" />
                      Edit
                    </button>

                    {station.status === 'online' && (
                      <button
                        onClick={() => handleStopBroadcast(station)}
                        className="flex-1 flex items-center justify-center gap-2 px-3 py-2 bg-orange-600 hover:bg-orange-700 rounded text-sm transition-colors"
                      >
                        <StopCircle className="w-4 h-4" />
                        Stop
                      </button>
                    )}
                  </div>

                  <button
                    onClick={() => handleDelete(station)}
                    className="w-full flex items-center justify-center gap-2 px-3 py-2 bg-red-600 hover:bg-red-700 rounded text-sm transition-colors"
                  >
                    <Trash2 className="w-4 h-4" />
                    Delete
                  </button>
                </>
              )}
            </div>
          </div>
        ))}
      </div>

      {/* Delete Confirmation Modal */}
      {deleteConfirmStation && (
        <div className="fixed inset-0 bg-black bg-opacity-50 flex items-center justify-center p-4 z-50">
          <div className="bg-gray-900 rounded-lg p-6 max-w-md w-full border border-gray-800">
            <h2 className="text-xl font-bold mb-4">Delete Station</h2>
            <p className="text-gray-400 mb-6">
              Are you sure you want to delete "{deleteConfirmStation.name}"? This action cannot be undone.
            </p>
            <div className="flex gap-3">
              <button
                onClick={() => setDeleteConfirmStation(null)}
                className="flex-1 py-2 px-4 bg-gray-800 hover:bg-gray-700 rounded transition-colors"
              >
                Cancel
              </button>
              <button
                onClick={() => handleDelete(deleteConfirmStation)}
                className="flex-1 py-2 px-4 bg-red-600 hover:bg-red-700 rounded transition-colors"
              >
                Delete
              </button>
            </div>
          </div>
        </div>
      )}

      {/* Create Station Modal */}
      <CreateStationModal
        isOpen={isCreateModalOpen}
        onClose={() => setIsCreateModalOpen(false)}
        onSuccess={handleStationCreated}
      />

      {/* Streaming Credentials Modal */}
      {credentialsStation && (
        <StreamingCredentialsModal
          isOpen={credentialsStation !== null}
          onClose={() => setCredentialsStation(null)}
          station={credentialsStation}
        />
      )}

      {/* Edit Station Modal */}
      {editStation && (
        <EditStationModal
          isOpen={editStation !== null}
          onClose={() => setEditStation(null)}
          onSuccess={() => {
            setEditStation(null);
            refetch();
          }}
          station={editStation}
        />
      )}
    </div>
  );
}
