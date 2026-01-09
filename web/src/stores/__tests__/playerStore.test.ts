import { describe, it, expect, beforeEach } from 'vitest';
import { usePlayerStore, Station } from '../playerStore';

describe('Player Store', () => {
  beforeEach(() => {
    // Reset store
    usePlayerStore.getState().stop();
  });

  const mockStation: Station = {
    id: 1,
    uuid: 'station-123',
    name: 'Test Radio',
    description: 'A test radio station',
    mountpoint: '/test-stream',
    status: 'online',
    listeners_count: 42,
    metadata_title: 'Now Playing',
    content_type: 'audio/mpeg',
    bitrate: 128,
    owner_pubkey: 'test-pubkey',
    is_private: false,
  };

  describe('Station Playback', () => {
    it('should play station', () => {
      const store = usePlayerStore.getState();
      store.playStation(mockStation);

      expect(store.currentStation).toEqual(mockStation);
      expect(store.isPlaying).toBe(true);
      expect(store.currentStreamUrl).toContain(mockStation.mountpoint);
    });

    it('should reset time when playing new station', () => {
      const store = usePlayerStore.getState();

      store.setCurrentTime(100);
      store.playStation(mockStation);

      expect(store.currentTime).toBe(0);
    });
  });

  describe('Playback Controls', () => {
    it('should pause playback', () => {
      const store = usePlayerStore.getState();

      store.playStation(mockStation);
      expect(store.isPlaying).toBe(true);

      store.pause();
      expect(store.isPlaying).toBe(false);
    });

    it('should resume playback', () => {
      const store = usePlayerStore.getState();

      store.playStation(mockStation);
      store.pause();
      expect(store.isPlaying).toBe(false);

      store.resume();
      expect(store.isPlaying).toBe(true);
    });

    it('should stop playback and clear state', () => {
      const store = usePlayerStore.getState();

      store.playStation(mockStation);
      store.setCurrentTime(100);
      store.stop();

      expect(store.isPlaying).toBe(false);
      expect(store.currentStation).toBeNull();
      expect(store.currentStreamUrl).toBeNull();
      expect(store.currentTime).toBe(0);
    });
  });

  describe('Volume Control', () => {
    it('should set volume', () => {
      const store = usePlayerStore.getState();

      store.setVolume(0.5);
      expect(store.volume).toBe(0.5);
    });

    it('should clamp volume to 0-1 range (prevent audio distortion)', () => {
      const store = usePlayerStore.getState();

      store.setVolume(1.5);
      expect(store.volume).toBe(1);

      store.setVolume(-0.5);
      expect(store.volume).toBe(0);
    });

    it('should handle edge cases', () => {
      const store = usePlayerStore.getState();

      store.setVolume(0);
      expect(store.volume).toBe(0);

      store.setVolume(1);
      expect(store.volume).toBe(1);
    });
  });

  describe('Time and Duration', () => {
    it('should set current time', () => {
      const store = usePlayerStore.getState();

      store.setCurrentTime(123.45);
      expect(store.currentTime).toBe(123.45);
    });

    it('should set duration', () => {
      const store = usePlayerStore.getState();

      store.setDuration(300);
      expect(store.duration).toBe(300);
    });

    it('should reset time on stop', () => {
      const store = usePlayerStore.getState();

      store.playStation(mockStation);
      store.setCurrentTime(100);
      store.stop();

      expect(store.currentTime).toBe(0);
    });
  });

  describe('Metadata', () => {
    it('should set metadata', () => {
      const store = usePlayerStore.getState();

      store.setMetadata('Artist - Song Title');
      expect(store.currentMetadata).toBe('Artist - Song Title');
    });

    it('should clear metadata', () => {
      const store = usePlayerStore.getState();

      store.setMetadata('Some metadata');
      store.setMetadata(null);

      expect(store.currentMetadata).toBeNull();
    });

    it('should clear metadata on stop', () => {
      const store = usePlayerStore.getState();

      store.playStation(mockStation);
      store.setMetadata('Test metadata');
      store.stop();

      expect(store.currentMetadata).toBeNull();
    });
  });

  describe('Security: URL Handling', () => {
    it('should use origin for station stream URLs', () => {
      const store = usePlayerStore.getState();
      store.playStation(mockStation);

      expect(store.currentStreamUrl).toContain(window.location.origin);
      expect(store.currentStreamUrl).toContain(mockStation.mountpoint);
    });
  });

  describe('State Transitions', () => {
    it('should maintain volume across playback', () => {
      const store = usePlayerStore.getState();

      store.setVolume(0.3);
      store.playStation(mockStation);
      expect(store.volume).toBe(0.3);

      store.stop();
      store.playStation(mockStation);
      expect(store.volume).toBe(0.3);
    });
  });
});
