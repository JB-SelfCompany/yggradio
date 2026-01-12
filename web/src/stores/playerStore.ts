import { create } from 'zustand';

export interface Station {
  id: number;
  uuid: string;
  name: string;
  description: string;
  mountpoint: string;
  status: 'online' | 'offline';
  listeners_count: number;
  metadata_title: string | null;
  content_type: string;
  bitrate: number | null;
  owner_pubkey: string;
  is_private: boolean;
  is_federated?: boolean; // True for federated stations from other nodes
  average_rating?: number; // Station rating (0-5)
  vote_count?: number; // Number of votes
  playlist_enabled?: boolean;
  playlist_directory?: string;
  playlist_mode?: 'sequential' | 'shuffle';
  playlist_loop?: boolean;
  playlist_file_pattern?: string;
  external_stream_url?: string | null; // URL of external stream (HLS or direct)
  external_stream_type?: 'hls' | 'direct' | null; // Type of external stream
  hls_proxy_url?: string | null; // Proxy URL for HLS streams (for browser playback)
}

interface PlayerState {
  // Playback state
  isPlaying: boolean;
  volume: number;
  currentTime: number;
  duration: number;

  // Current content
  currentStation: Station | null;
  currentStreamUrl: string | null;

  // Metadata
  currentMetadata: string | null;

  // Actions
  playStation: (station: Station) => void;
  pause: () => void;
  resume: () => void;
  stop: () => void;
  setVolume: (volume: number) => void;
  setCurrentTime: (time: number) => void;
  setDuration: (duration: number) => void;
  setMetadata: (metadata: string | null) => void;
}

export const usePlayerStore = create<PlayerState>((set) => ({
  isPlaying: false,
  volume: 0.7,
  currentTime: 0,
  duration: 0,

  currentStation: null,
  currentStreamUrl: null,

  currentMetadata: null,

  playStation: (station: Station) => {
    // For HLS external streams, use proxy URL if available
    let streamUrl: string;
    if (station.external_stream_type === 'hls' && station.hls_proxy_url) {
      streamUrl = `${window.location.origin}${station.hls_proxy_url}`;
    } else {
      streamUrl = `${window.location.origin}${station.mountpoint}`;
    }

    set({
      currentStation: station,
      currentStreamUrl: streamUrl,
      isPlaying: true,
      currentTime: 0,
      duration: 0,
      currentMetadata: null, // Reset metadata when switching stations
    });
  },

  pause: () => set({ isPlaying: false }),

  resume: () => set({ isPlaying: true }),

  stop: () =>
    set({
      isPlaying: false,
      currentTime: 0,
      currentStation: null,
      currentStreamUrl: null,
      currentMetadata: null,
    }),

  setVolume: (volume: number) => {
    const clampedVolume = Math.max(0, Math.min(1, volume));
    set({ volume: clampedVolume });
  },

  setCurrentTime: (time: number) => set({ currentTime: time }),

  setDuration: (duration: number) => set({ duration }),

  setMetadata: (metadata: string | null) => set({ currentMetadata: metadata }),
}));
