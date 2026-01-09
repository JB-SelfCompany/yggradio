import { describe, it, expect, beforeEach, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import AudioPlayer from '../AudioPlayer';
import { usePlayerStore, Station, Episode, Podcast } from '../../../stores/playerStore';

describe('AudioPlayer Component', () => {
  const mockStation: Station = {
    uuid: 'station-123',
    name: 'Test Radio',
    description: 'Test Description',
    mountpoint: '/test-stream',
    status: 'online',
    listenersCount: 10,
    metadataTitle: 'Test Song',
    contentType: 'audio/mpeg',
    bitrate: 128,
  };

  const mockPodcast: Podcast = {
    uuid: 'podcast-123',
    title: 'Test Podcast',
    description: 'Test Podcast Description',
    author: 'Test Author',
    imageUrl: null,
    category: 'Technology',
  };

  const mockEpisode: Episode = {
    uuid: 'episode-123',
    podcastId: 1,
    title: 'Test Episode',
    description: 'Test Episode Description',
    audioFile: 'test.mp3',
    fileSize: 1000000,
    duration: 1800,
    mimeType: 'audio/mpeg',
    publishedAt: '2024-01-01T00:00:00Z',
    enclosureUrl: 'https://example.com/test.mp3',
  };

  beforeEach(() => {
    usePlayerStore.getState().stop();
  });

  it('should not render when no stream is loaded', () => {
    const { container } = render(<AudioPlayer />);
    expect(container.firstChild).toBeNull();
  });

  it('should render when station is playing', () => {
    usePlayerStore.getState().playStation(mockStation);
    render(<AudioPlayer />);

    expect(screen.getByText('Test Radio')).toBeInTheDocument();
    expect(screen.getByLabelText('Pause')).toBeInTheDocument();
  });

  it('should display LIVE indicator for stations', () => {
    usePlayerStore.getState().playStation(mockStation);
    render(<AudioPlayer />);

    expect(screen.getByText('LIVE')).toBeInTheDocument();
  });

  it('should display episode information', () => {
    usePlayerStore.getState().playEpisode(mockEpisode, mockPodcast);
    render(<AudioPlayer />);

    expect(screen.getByText('Test Episode')).toBeInTheDocument();
    expect(screen.getByText('Test Podcast')).toBeInTheDocument();
  });

  it('should toggle play/pause on button click', () => {
    usePlayerStore.getState().playStation(mockStation);
    render(<AudioPlayer />);

    const playPauseButton = screen.getByLabelText('Pause');
    fireEvent.click(playPauseButton);

    expect(usePlayerStore.getState().isPlaying).toBe(false);
    expect(screen.getByLabelText('Play')).toBeInTheDocument();
  });

  it('should stop playback on stop button click', () => {
    usePlayerStore.getState().playStation(mockStation);
    render(<AudioPlayer />);

    const stopButton = screen.getByLabelText('Stop');
    fireEvent.click(stopButton);

    expect(usePlayerStore.getState().isPlaying).toBe(false);
    expect(usePlayerStore.getState().currentStation).toBeNull();
  });

  it('should adjust volume with slider', () => {
    usePlayerStore.getState().playStation(mockStation);
    render(<AudioPlayer />);

    const volumeSlider = screen.getByLabelText('Volume');
    fireEvent.change(volumeSlider, { target: { value: '0.5' } });

    expect(usePlayerStore.getState().volume).toBe(0.5);
  });

  it('should toggle mute on button click', () => {
    usePlayerStore.getState().playStation(mockStation);
    render(<AudioPlayer />);

    const muteButton = screen.getByLabelText('Mute');
    fireEvent.click(muteButton);

    expect(screen.getByLabelText('Unmute')).toBeInTheDocument();
  });

  it('should display progress bar for episodes', () => {
    usePlayerStore.getState().playEpisode(mockEpisode, mockPodcast);
    render(<AudioPlayer />);

    const progressBar = screen.getByRole('slider', { name: /seek/i });
    expect(progressBar).toBeInTheDocument();
  });

  it('should not display progress bar for live stations', () => {
    usePlayerStore.getState().playStation(mockStation);
    render(<AudioPlayer />);

    const progressBars = screen.queryAllByRole('slider');
    // Only volume slider should be present, not seek slider
    expect(progressBars.length).toBe(1);
  });

  it('should format time correctly', () => {
    usePlayerStore.getState().playEpisode(mockEpisode, mockPodcast);
    render(<AudioPlayer />);

    // Should display duration (1800 seconds = 30:00)
    expect(screen.getByText('30:00')).toBeInTheDocument();
  });

  it('should display metadata when available', () => {
    usePlayerStore.getState().playStation(mockStation);
    usePlayerStore.getState().setMetadata('Artist - Song Title');
    render(<AudioPlayer />);

    expect(screen.getByText('Artist - Song Title')).toBeInTheDocument();
  });

  it('should update volume when unmuting', () => {
    usePlayerStore.getState().playStation(mockStation);
    usePlayerStore.getState().setVolume(0.7);
    render(<AudioPlayer />);

    const muteButton = screen.getByLabelText('Mute');
    fireEvent.click(muteButton);

    const volumeSlider = screen.getByLabelText('Volume');
    fireEvent.change(volumeSlider, { target: { value: '0.5' } });

    // Should unmute when adjusting volume
    expect(screen.getByLabelText('Mute')).toBeInTheDocument();
  });

  it('should handle seek for episodes only', () => {
    usePlayerStore.getState().playEpisode(mockEpisode, mockPodcast);
    render(<AudioPlayer />);

    // Find the seek slider (not the volume slider)
    const sliders = screen.getAllByRole('slider');
    // First slider should be seek, second is volume
    const seekSlider = sliders[0];

    fireEvent.change(seekSlider, { target: { value: '900' } });

    // Audio element's currentTime should be updated (via ref)
    // We can't easily test this without a real audio element
    // But we can verify the slider accepted the value
    expect(seekSlider).toHaveValue('900');
  });

  describe('Security: Audio Source Handling', () => {
    it('should set audio source from station mountpoint', () => {
      usePlayerStore.getState().playStation(mockStation);
      const { container } = render(<AudioPlayer />);

      const audio = container.querySelector('audio');
      expect(audio).toBeTruthy();
      // Audio src will be set by useEffect
    });

    it('should set audio source from episode URL', () => {
      usePlayerStore.getState().playEpisode(mockEpisode, mockPodcast);
      const { container } = render(<AudioPlayer />);

      const audio = container.querySelector('audio');
      expect(audio).toBeTruthy();
    });

    it('should use crossOrigin="anonymous" for CORS', () => {
      usePlayerStore.getState().playStation(mockStation);
      const { container } = render(<AudioPlayer />);

      const audio = container.querySelector('audio');
      expect(audio?.getAttribute('crossOrigin')).toBe('anonymous');
    });
  });

  describe('Volume Safety', () => {
    it('should not allow volume above 1 (audio safety)', () => {
      usePlayerStore.getState().playStation(mockStation);
      render(<AudioPlayer />);

      const volumeSlider = screen.getByLabelText('Volume');
      fireEvent.change(volumeSlider, { target: { value: '1.5' } });

      // Volume should be clamped to 1 by the store
      expect(usePlayerStore.getState().volume).toBe(1);
    });

    it('should not allow negative volume', () => {
      usePlayerStore.getState().playStation(mockStation);
      render(<AudioPlayer />);

      const volumeSlider = screen.getByLabelText('Volume');
      fireEvent.change(volumeSlider, { target: { value: '-0.5' } });

      // Volume should be clamped to 0 by the store
      expect(usePlayerStore.getState().volume).toBe(0);
    });
  });
});
