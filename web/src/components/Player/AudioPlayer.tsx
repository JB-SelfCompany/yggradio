import { useEffect, useRef, useState } from 'react';
import { usePlayerStore } from '../../stores/playerStore';
import { Play, Pause, Square, Volume2, VolumeX } from 'lucide-react';
import { MSEAudioPlayer, detectMimeType } from '../../lib/mse-player';
import { isYggdrasilStream } from '../../lib/utils';
import Hls from 'hls.js';

export default function AudioPlayer() {
  const audioRef = useRef<HTMLAudioElement>(null);
  const playerRef = useRef<MSEAudioPlayer | null>(null);
  const hlsRef = useRef<Hls | null>(null);
  const [isMuted, setIsMuted] = useState(false);
  const [bufferStatus, setBufferStatus] = useState<string>('Connecting...');
  const errorRetryCountRef = useRef<number>(0);
  const errorRetryTimeoutRef = useRef<number | null>(null);
  const lastStreamUrlRef = useRef<string>('');
  const isDirectStreamRef = useRef<boolean>(false);
  const bufferCleanupIntervalRef = useRef<number | null>(null);
  const streamStartTimeRef = useRef<number>(0);

  const {
    isPlaying,
    volume,
    currentStation,
    currentStreamUrl,
    currentMetadata,
    pause,
    resume,
    stop,
    setVolume,
    setCurrentTime,
    setDuration,
    setMetadata,
  } = usePlayerStore();

  // Audio element event handlers
  useEffect(() => {
    const audio = audioRef.current;
    if (!audio) return;

    const handleTimeUpdate = () => {
      setCurrentTime(audio.currentTime);
    };

    const handleDurationChange = () => {
      setDuration(audio.duration);
    };

    const handleEnded = () => {
      stop();
    };

    const handleError = () => {
      const audio = audioRef.current;
      if (!audio) return;

      const error = audio.error;
      if (!error) return;

      console.error('Audio playback error:', {
        code: error.code,
        message: error.message,
        MEDIA_ERR_ABORTED: error.code === 1,
        MEDIA_ERR_NETWORK: error.code === 2,
        MEDIA_ERR_DECODE: error.code === 3,
        MEDIA_ERR_SRC_NOT_SUPPORTED: error.code === 4,
      });

      // Try to recover from decode errors for all non-HLS streams
      if (error.code === 3 && isPlaying) {
        // MEDIA_ERR_DECODE - попытаться переподключиться
        const maxRetries = 5; // Увеличил до 5 попыток

        if (errorRetryCountRef.current < maxRetries) {
          errorRetryCountRef.current += 1;
          const retryDelay = 1000 * errorRetryCountRef.current; // 1s, 2s, 3s, 4s, 5s

          console.log(`Decode error detected, retrying in ${retryDelay}ms (attempt ${errorRetryCountRef.current}/${maxRetries})`);

          // Очистить предыдущий таймер если есть
          if (errorRetryTimeoutRef.current) {
            clearTimeout(errorRetryTimeoutRef.current);
          }

          errorRetryTimeoutRef.current = setTimeout(() => {
            if (!audio) return;

            console.log('Reconnecting to stream...');

            // Для MSE player - нужно пересоздать
            if (playerRef.current) {
              console.log('Recreating MSE player...');
              playerRef.current.destroy();
              playerRef.current = null;

              // Перезапустить через обновление URL
              const currentUrl = lastStreamUrlRef.current || audio.dataset['streamUrl'];
              if (currentUrl) {
                audio.src = '';
                audio.load();

                // Небольшая задержка перед перезапуском
                setTimeout(() => {
                  // Триггерим перезагрузку через изменение src
                  audio.src = currentUrl;
                  audio.load();
                  audio.play().catch((err) => {
                    console.error('Failed to reconnect MSE:', err);
                  });
                }, 100);
              }
            } else if (lastStreamUrlRef.current || audio.src) {
              // Для direct потоков - просто перезагрузить
              const streamUrl = lastStreamUrlRef.current || audio.src;
              audio.src = streamUrl;
              audio.load();
              audio.play().catch((err) => {
                console.error('Failed to reconnect:', err);
                pause();
              });
            }
          }, retryDelay);
        } else {
          console.error('Maximum retry attempts reached, stopping playback');
          pause();
        }
      } else {
        // Для других типов ошибок - просто остановить
        pause();
      }
    };

    const handleWaiting = () => {
      // Audio is buffering
    };

    const handleCanPlay = () => {
      // Audio can play
    };

    const handlePlaying = () => {
      // Audio is playing - сбросить счетчик ошибок при успешном воспроизведении
      errorRetryCountRef.current = 0;
      if (errorRetryTimeoutRef.current) {
        clearTimeout(errorRetryTimeoutRef.current);
        errorRetryTimeoutRef.current = null;
      }
    };

    const handleStalled = () => {
      // Audio stream stalled
    };

    audio.addEventListener('timeupdate', handleTimeUpdate);
    audio.addEventListener('durationchange', handleDurationChange);
    audio.addEventListener('ended', handleEnded);
    audio.addEventListener('error', handleError as EventListener);
    audio.addEventListener('waiting', handleWaiting);
    audio.addEventListener('canplay', handleCanPlay);
    audio.addEventListener('playing', handlePlaying);
    audio.addEventListener('stalled', handleStalled);

    return () => {
      audio.removeEventListener('timeupdate', handleTimeUpdate);
      audio.removeEventListener('durationchange', handleDurationChange);
      audio.removeEventListener('ended', handleEnded);
      audio.removeEventListener('error', handleError as EventListener);
      audio.removeEventListener('waiting', handleWaiting);
      audio.removeEventListener('canplay', handleCanPlay);
      audio.removeEventListener('playing', handlePlaying);
      audio.removeEventListener('stalled', handleStalled);
    };
  }, [pause, stop, setCurrentTime, setDuration]);

  // Update audio source when stream URL changes
  useEffect(() => {
    const audio = audioRef.current;
    if (!audio || !currentStreamUrl) return;

    // Reset error retry counter when switching streams
    errorRetryCountRef.current = 0;
    if (errorRetryTimeoutRef.current) {
      clearTimeout(errorRetryTimeoutRef.current);
      errorRetryTimeoutRef.current = null;
    }
    isDirectStreamRef.current = false;
    lastStreamUrlRef.current = '';

    // Cleanup previous players
    if (playerRef.current) {
      playerRef.current.destroy();
      playerRef.current = null;
    }
    if (hlsRef.current) {
      hlsRef.current.destroy();
      hlsRef.current = null;
    }

    // Check if this is an HLS external stream (use proxy URL)
    const isExternalHLS = currentStation?.external_stream_type === 'hls' &&
                          currentStation?.hls_proxy_url;

    // MSE (MediaSource Extensions) does NOT support audio/mpeg (MP3) in most browsers
    // So we can only use MSE for direct Yggdrasil IPv6 streams with supported codecs (Opus/AAC)
    // Federated streams use direct playback since they're typically MP3
    const isYggdrasil = isYggdrasilStream(currentStreamUrl);
    const isFederatedYggdrasil = isYggdrasil && currentStation?.is_federated;

    if (isExternalHLS && Hls.isSupported()) {
      // Use HLS.js for external HLS streams via proxy
      const hls = new Hls({
        debug: false,
        enableWorker: true,
        lowLatencyMode: false,
        backBufferLength: 10, // Aggressive: keep only 10s behind playhead to prevent memory leak
        maxBufferLength: 15,  // Aggressive: buffer only 15s ahead
        maxMaxBufferLength: 20, // Aggressive: absolute max 20s
      });

      hlsRef.current = hls;

      // Use the proxy URL from currentStreamUrl (already set in playerStore)
      hls.loadSource(currentStreamUrl);
      hls.attachMedia(audio);

      hls.on(Hls.Events.MANIFEST_PARSED, () => {
        if (isPlaying) {
          audio.play().catch((error) => {
            console.error('Failed to play HLS audio:', error);
            pause();
          });
        }
      });

      hls.on(Hls.Events.ERROR, (_event, data) => {
        if (data.fatal) {
          switch (data.type) {
            case Hls.ErrorTypes.NETWORK_ERROR:
              // Network error, trying to recover
              hls.startLoad();
              break;
            case Hls.ErrorTypes.MEDIA_ERROR:
              // Media error, trying to recover
              hls.recoverMediaError();
              break;
            default:
              // Fatal error, cannot recover
              console.error('HLS fatal error:', data.type);
              pause();
              break;
          }
        }
      });
    } else if (isFederatedYggdrasil && currentStation) {
      // MSE player only for direct Yggdrasil streams with supported codecs
      // Note: audio/mpeg (MP3) is NOT supported by MSE in most browsers!
      const contentType = currentStation.content_type || 'audio/mpeg';
      const mimeType = detectMimeType(contentType);

      // Check if MSE actually supports this MIME type
      if (MediaSource.isTypeSupported(mimeType)) {
        lastStreamUrlRef.current = currentStreamUrl;
        playerRef.current = new MSEAudioPlayer(audio, mimeType);
        audio.dataset['streamUrl'] = currentStreamUrl;
        playerRef.current.setMinBufferSeconds(3.0);
      } else {
        // Fallback to direct playback for unsupported codecs (like MP3)
        console.log(`MSE does not support ${mimeType}, using direct playback`);
        isDirectStreamRef.current = true;
        lastStreamUrlRef.current = currentStreamUrl;
        streamStartTimeRef.current = Date.now();
        audio.src = currentStreamUrl;
        audio.load();

        if (isPlaying) {
          audio.play().catch((error) => {
            console.error('Failed to play audio:', error);
            pause();
          });
        }
      }
    } else {
      // Direct playback for regular internet or native HLS support
      isDirectStreamRef.current = true;
      lastStreamUrlRef.current = currentStreamUrl;
      streamStartTimeRef.current = Date.now();
      audio.src = currentStreamUrl;
      audio.load();

      if (isPlaying) {
        audio.play().catch((error) => {
          console.error('Failed to play audio:', error);
          pause();
        });
      }

      // For direct streams: periodically reload to prevent Firefox memory leak
      // Check every 30 minutes if stream has been playing for 2+ hours
      if (bufferCleanupIntervalRef.current) {
        clearInterval(bufferCleanupIntervalRef.current);
      }

      bufferCleanupIntervalRef.current = setInterval(() => {
        const elapsedMs = Date.now() - streamStartTimeRef.current;
        const elapsedHours = elapsedMs / (1000 * 60 * 60);

        // Reload stream every 2 hours to clear browser buffers
        if (elapsedHours >= 2 && isPlaying && audio.src === currentStreamUrl) {
          console.log('Direct stream: reloading to clear buffers (played for 2+ hours)');
          const currentTime = audio.currentTime;
          audio.load();
          audio.currentTime = currentTime;
          audio.play().catch((error) => {
            console.error('Failed to reload stream:', error);
          });
          streamStartTimeRef.current = Date.now(); // Reset timer
        }
      }, 30 * 60 * 1000); // Check every 30 minutes
    }

    return () => {
      if (playerRef.current) {
        playerRef.current.destroy();
        playerRef.current = null;
      }
      if (hlsRef.current) {
        hlsRef.current.destroy();
        hlsRef.current = null;
      }
      if (errorRetryTimeoutRef.current) {
        clearTimeout(errorRetryTimeoutRef.current);
        errorRetryTimeoutRef.current = null;
      }
      if (bufferCleanupIntervalRef.current) {
        clearInterval(bufferCleanupIntervalRef.current);
        bufferCleanupIntervalRef.current = null;
      }
    };
  }, [currentStreamUrl, currentStation, isPlaying, pause]);

  // Control playback
  useEffect(() => {
    const audio = audioRef.current;
    if (!audio) return;

    if (isPlaying) {
      audio.play().catch((error) => {
        console.error('Failed to play audio:', error);
        pause();
      });
    } else {
      audio.pause();
      // При остановке воспроизведения - сбросить счетчик ошибок и очистить таймеры
      errorRetryCountRef.current = 0;
      if (errorRetryTimeoutRef.current) {
        clearTimeout(errorRetryTimeoutRef.current);
        errorRetryTimeoutRef.current = null;
      }
      if (bufferCleanupIntervalRef.current) {
        clearInterval(bufferCleanupIntervalRef.current);
        bufferCleanupIntervalRef.current = null;
      }
    }
  }, [isPlaying, pause]);

  // Update volume
  useEffect(() => {
    const audio = audioRef.current;
    if (!audio) return;

    audio.volume = isMuted ? 0 : volume;
  }, [volume, isMuted]);

  // Poll for metadata updates when playing a station
  useEffect(() => {
    if (!currentStation || !isPlaying) return;

    // Initial metadata
    if (currentStation.metadata_title) {
      setMetadata(currentStation.metadata_title);
    }

    // Poll every 30 seconds for metadata updates (reduced from 10s to minimize browser logs)
    const pollInterval = setInterval(async () => {
      try {
        // For federated stations, we need to fetch from the stations list
        // because mountpoint format is /stream/federated/{node}/{mount}
        if (currentStation.is_federated) {
          const response = await fetch('/api/stations');
          if (response.ok) {
            const data = await response.json();
            const updatedStation = data.stations?.find(
              (s: typeof currentStation) => s.uuid === currentStation.uuid
            );
            if (updatedStation?.metadata_title) {
              setMetadata(updatedStation.metadata_title);
            }
          }
        } else {
          // For local stations, use the mount endpoint
          const response = await fetch(`/api/stations/mount${currentStation.mountpoint}`);
          if (response.ok) {
            const stationData = await response.json();
            if (stationData.metadata_title) {
              setMetadata(stationData.metadata_title);
            }
          }
        }
      } catch (error) {
        // Silently fail - metadata updates are not critical
      }
    }, 30000);

    return () => clearInterval(pollInterval);
  }, [currentStation, isPlaying, setMetadata]);

  // Update document title and Media Session API when metadata changes
  useEffect(() => {
    const defaultTitle = 'YggRadio';

    if (!currentStreamUrl) {
      document.title = defaultTitle;
      return;
    }

    // Parse metadata for title display
    const parseMetadataForTitle = (metadata: string | null) => {
      if (!metadata) return null;
      const parts = metadata.split(' - ');
      if (parts.length >= 2 && parts[0]) {
        return {
          artist: parts[0].trim(),
          title: parts.slice(1).join(' - ').trim(),
        };
      }
      return { artist: null, title: metadata };
    };

    const metadataParts = parseMetadataForTitle(currentMetadata);

    // Update document title
    if (currentStation) {
      if (currentMetadata && metadataParts) {
        if (metadataParts.artist && metadataParts.title) {
          document.title = `${metadataParts.artist} - ${metadataParts.title} • ${currentStation.name} • ${defaultTitle}`;
        } else {
          document.title = `${currentMetadata} • ${currentStation.name} • ${defaultTitle}`;
        }
      } else {
        document.title = `${currentStation.name} • ${defaultTitle}`;
      }
    } else {
      document.title = defaultTitle;
    }

    // Update Media Session API if available
    if ('mediaSession' in navigator) {
      const metadata: {
        title: string;
        artist?: string;
        album?: string;
        artwork?: { src: string; sizes: string; type: string }[];
      } = {
        title: currentStation?.name || 'Unknown',
      };

      if (currentStation && metadataParts) {
        if (metadataParts.artist && metadataParts.title) {
          metadata.title = metadataParts.title;
          metadata.artist = metadataParts.artist;
          metadata.album = currentStation.name;
        } else if (currentMetadata) {
          metadata.title = currentMetadata;
          metadata.artist = currentStation.name;
        } else {
          metadata.title = currentStation.name;
          metadata.artist = 'Live Radio';
        }
      }

      try {
        navigator.mediaSession.metadata = new MediaMetadata(metadata);
      } catch (error) {
        console.error('Failed to update Media Session metadata:', error);
      }
    }

    // Cleanup on unmount
    return () => {
      document.title = defaultTitle;
      if ('mediaSession' in navigator) {
        navigator.mediaSession.metadata = null;
      }
    };
  }, [currentMetadata, currentStation, currentStreamUrl]);

  // Monitor buffer status for MSE player
  useEffect(() => {
    if (!playerRef.current) return;

    const interval = setInterval(() => {
      if (playerRef.current) {
        const buffered = playerRef.current.getBufferedSeconds();
        setBufferStatus(`Buffered: ${buffered.toFixed(1)}s`);
      }
    }, 500);

    return () => clearInterval(interval);
  }, [playerRef.current]);

  const handlePlayPause = () => {
    if (isPlaying) {
      pause();
    } else {
      resume();
    }
  };

  const handleStop = () => {
    stop();
  };

  const handleVolumeChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    const newVolume = parseFloat(e.target.value);
    setVolume(newVolume);
    if (newVolume > 0) {
      setIsMuted(false);
    }
  };

  const toggleMute = () => {
    setIsMuted(!isMuted);
  };

  // Don't render if nothing is loaded
  if (!currentStreamUrl) {
    return null;
  }

  // For radio stations: always show track info on top, station name on bottom
  // Top: track title, or "Now Playing" header with "No metadata" if unavailable
  // Bottom: station name
  const displayTitle = currentMetadata || 'No metadata';
  const displaySubtitle = currentStation?.name || 'Unknown Station';
  const hasNoMetadata = !currentMetadata;

  return (
    <div className="fixed bottom-0 left-0 right-0 bg-gray-900 border-t border-gray-700 shadow-lg z-50">
      <audio
        ref={audioRef}
        preload="auto"
        crossOrigin="anonymous"
      />

      <div className="container mx-auto px-2 sm:px-4 py-2 sm:py-3">
        {/* Mobile Layout (< 768px): Two rows */}
        <div className="md:hidden">
          {/* Row 1: Controls + Track Info + Live indicator */}
          <div className="flex items-center gap-2 mb-2">
            {/* Playback Controls */}
            <button
              onClick={handlePlayPause}
              className="p-2 rounded-full bg-indigo-600 hover:bg-indigo-700 transition-colors flex-shrink-0"
              aria-label={isPlaying ? 'Pause' : 'Play'}
            >
              {isPlaying ? (
                <Pause className="w-5 h-5" />
              ) : (
                <Play className="w-5 h-5 ml-0.5" />
              )}
            </button>

            {/* Track Info */}
            <div className="flex-1 min-w-0 px-2">
              {hasNoMetadata ? (
                <>
                  <div className="text-xs text-gray-400">Now Playing</div>
                  <div className="font-semibold text-white text-sm break-words line-clamp-2">
                    {displayTitle}
                  </div>
                </>
              ) : (
                <div className="font-semibold text-white text-sm break-words line-clamp-2">
                  {displayTitle}
                </div>
              )}
              {displaySubtitle && (
                <div className="text-xs text-gray-400 truncate">
                  {displaySubtitle}
                </div>
              )}
            </div>

            {/* Live indicator for stations */}
            {currentStation && (
              <div className="flex items-center gap-1 flex-shrink-0">
                <div className="w-1.5 h-1.5 bg-red-500 rounded-full animate-pulse" />
                <span className="text-xs text-gray-400">LIVE</span>
              </div>
            )}
          </div>

          {/* Row 2: Volume Control */}
          <div className="flex items-center gap-2 px-1">
            <button
              onClick={toggleMute}
              className="p-1 rounded hover:bg-gray-700 transition-colors flex-shrink-0"
              aria-label={isMuted ? 'Unmute' : 'Mute'}
            >
              {isMuted || volume === 0 ? (
                <VolumeX className="w-4 h-4" />
              ) : (
                <Volume2 className="w-4 h-4" />
              )}
            </button>
            <input
              type="range"
              min="0"
              max="1"
              step="0.01"
              value={isMuted ? 0 : volume}
              onChange={handleVolumeChange}
              className="flex-1 h-2 bg-gray-700 rounded-lg appearance-none cursor-pointer accent-indigo-600"
              aria-label="Volume"
            />
            <span className="text-xs text-gray-400 tabular-nums flex-shrink-0 w-8 text-right">
              {Math.round((isMuted ? 0 : volume) * 100)}%
            </span>
          </div>
        </div>

        {/* Desktop Layout (>= 768px): Single row */}
        <div className="hidden md:flex items-center gap-4">
          {/* Playback Controls */}
          <div className="flex items-center gap-2">
            <button
              onClick={handlePlayPause}
              className="p-2 rounded-full bg-indigo-600 hover:bg-indigo-700 transition-colors"
              aria-label={isPlaying ? 'Pause' : 'Play'}
            >
              {isPlaying ? (
                <Pause className="w-6 h-6" />
              ) : (
                <Play className="w-6 h-6 ml-0.5" />
              )}
            </button>

            <button
              onClick={handleStop}
              className="p-2 rounded-full bg-gray-700 hover:bg-gray-600 transition-colors"
              aria-label="Stop"
            >
              <Square className="w-6 h-6" />
            </button>
          </div>

          {/* Track Info */}
          <div className="flex-1 min-w-0">
            {hasNoMetadata ? (
              <>
                <div className="text-xs text-gray-400">Now Playing</div>
                <div className="font-semibold text-white truncate">
                  {displayTitle}
                </div>
              </>
            ) : (
              <div className="font-semibold text-white truncate">
                {displayTitle}
              </div>
            )}
            {displaySubtitle && (
              <div className="text-sm text-gray-400 truncate">
                {displaySubtitle}
              </div>
            )}
          </div>

          {/* Live indicator for stations */}
          {currentStation && (
            <div className="flex items-center gap-2">
              <div className="w-2 h-2 bg-red-500 rounded-full animate-pulse" />
              <span className="text-sm text-gray-400">LIVE</span>
              {/* Show buffer status for Yggdrasil streams using MSE */}
              {playerRef.current && (
                <span className="text-xs text-gray-500 ml-2">
                  {bufferStatus}
                </span>
              )}
            </div>
          )}

          {/* Volume Control */}
          <div className="flex items-center gap-2 w-32">
            <button
              onClick={toggleMute}
              className="p-2 rounded hover:bg-gray-700 transition-colors"
              aria-label={isMuted ? 'Unmute' : 'Mute'}
            >
              {isMuted || volume === 0 ? (
                <VolumeX className="w-5 h-5" />
              ) : (
                <Volume2 className="w-5 h-5" />
              )}
            </button>
            <input
              type="range"
              min="0"
              max="1"
              step="0.01"
              value={isMuted ? 0 : volume}
              onChange={handleVolumeChange}
              className="flex-1 h-2 bg-gray-700 rounded-lg appearance-none cursor-pointer accent-indigo-600"
              aria-label="Volume"
            />
          </div>
        </div>
      </div>
    </div>
  );
}
