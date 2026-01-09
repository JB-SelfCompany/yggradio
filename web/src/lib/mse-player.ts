/**
 * MediaSource Extensions Audio Player
 * Provides full control over buffering for high-latency networks like Yggdrasil
 *
 * This player manually controls buffering and starts playback as soon as
 * sufficient data is available (1-2 seconds), instead of waiting for
 * browser's default prebuffer (which can be 30+ seconds).
 */

export class MSEAudioPlayer {
  private mediaSource: MediaSource;
  private sourceBuffer: SourceBuffer | null = null;
  private audio: HTMLAudioElement;
  private fetchController: AbortController;
  private mimeType: string;
  private isPlaying: boolean = false;
  private minBufferSeconds: number = 1.0; // Start playing after 1 second buffered

  constructor(audioElement: HTMLAudioElement, mimeType: string = 'audio/mpeg') {
    this.audio = audioElement;
    this.mimeType = mimeType;
    this.mediaSource = new MediaSource();
    this.audio.src = URL.createObjectURL(this.mediaSource);
    this.fetchController = new AbortController();

    this.mediaSource.addEventListener('sourceopen', () => {
      this.onSourceOpen();
    });

    this.mediaSource.addEventListener('sourceended', () => {
      console.log('MSE: Source ended');
    });

    this.mediaSource.addEventListener('sourceclose', () => {
      console.log('MSE: Source closed');
    });
  }

  private onSourceOpen() {
    try {
      // CRITICAL: Specify correct MIME type
      // For MP3: 'audio/mpeg'
      // For Opus: 'audio/webm; codecs="opus"'
      // For AAC: 'audio/mp4; codecs="mp4a.40.2"'

      if (!MediaSource.isTypeSupported(this.mimeType)) {
        console.error(`MSE: MIME type not supported: ${this.mimeType}`);
        this.fallbackToDirectPlay();
        return;
      }

      this.sourceBuffer = this.mediaSource.addSourceBuffer(this.mimeType);

      // CRITICAL: Use 'sequence' mode for live streams
      this.sourceBuffer.mode = 'sequence';

      console.log('MSE: SourceBuffer created successfully');

      this.startFetching();
    } catch (error) {
      console.error('MSE: Error creating SourceBuffer:', error);
      this.fallbackToDirectPlay();
    }
  }

  private async startFetching() {
    const streamUrl = this.audio.dataset['streamUrl'];
    if (!streamUrl) {
      console.error('MSE: No stream URL provided');
      return;
    }

    console.log('MSE: Starting fetch from', streamUrl);

    try {
      const response = await fetch(streamUrl, {
        signal: this.fetchController.signal,
        headers: {
          'Cache-Control': 'no-cache',
        }
      });

      if (!response.ok) {
        throw new Error(`HTTP error ${response.status}`);
      }

      if (!response.body) {
        throw new Error('No response body');
      }

      const reader = response.body.getReader();

      // For Yggdrasil: accumulate 8KB chunks before appending to SourceBuffer
      const chunkSize = 8192;
      let buffer: Uint8Array<ArrayBuffer> = new Uint8Array(new ArrayBuffer(0));
      let totalReceived = 0;

      while (true) {
        const { done, value } = await reader.read();

        if (done) {
          console.log('MSE: Stream ended, total received:', totalReceived);
          break;
        }

        // Accumulate data - create a copy to avoid SharedArrayBuffer type issues
        const uint8Value: Uint8Array = new Uint8Array(new ArrayBuffer(value.length));
        uint8Value.set(value);
        buffer = this.concatArrays(buffer, uint8Value);
        totalReceived += value.length;

        // When we have enough data AND SourceBuffer is ready, append it
        if (buffer.length >= chunkSize && this.sourceBuffer && !this.sourceBuffer.updating) {
          const chunk = buffer.slice(0, chunkSize);
          buffer = buffer.slice(chunkSize);

          try {
            this.sourceBuffer.appendBuffer(chunk);

            // CRITICAL: Start playback as soon as we have minimum buffer
            this.checkAndStartPlayback();

          } catch (error) {
            console.error('MSE: Error appending buffer:', error);
            break;
          }
        }
      }

      // Send remaining buffer
      if (buffer.length > 0 && this.sourceBuffer && !this.sourceBuffer.updating) {
        this.sourceBuffer.appendBuffer(buffer);
      }

    } catch (error) {
      if ((error as Error).name === 'AbortError') {
        console.log('MSE: Fetch aborted');
      } else {
        console.error('MSE: Streaming error:', error);
        this.fallbackToDirectPlay();
      }
    }
  }

  private checkAndStartPlayback() {
    if (this.isPlaying || !this.sourceBuffer) return;

    if (this.audio.paused && this.sourceBuffer.buffered.length > 0) {
      const bufferedEnd = this.sourceBuffer.buffered.end(0);
      const bufferedSeconds = bufferedEnd - this.audio.currentTime;

      console.log(`MSE: Buffered ${bufferedSeconds.toFixed(2)} seconds`);

      // Start playing when we have minimum buffer
      if (bufferedSeconds >= this.minBufferSeconds) {
        console.log('MSE: Starting playback');
        this.audio.play()
          .then(() => {
            this.isPlaying = true;
            console.log('MSE: Playback started successfully');
          })
          .catch(error => {
            console.error('MSE: Error starting playback:', error);
          });
      }
    }
  }

  private concatArrays(a: Uint8Array, b: Uint8Array): Uint8Array<ArrayBuffer> {
    const result = new Uint8Array(new ArrayBuffer(a.length + b.length));
    result.set(a, 0);
    result.set(b, a.length);
    return result;
  }

  private fallbackToDirectPlay() {
    console.log('MSE: Falling back to direct audio element playback');
    const streamUrl = this.audio.dataset['streamUrl'];
    if (streamUrl) {
      this.audio.src = streamUrl;
      this.audio.load();
    }
  }

  public setMinBufferSeconds(seconds: number) {
    this.minBufferSeconds = seconds;
  }

  public destroy() {
    console.log('MSE: Destroying player');
    this.fetchController.abort();

    if (this.sourceBuffer && this.mediaSource.readyState === 'open') {
      try {
        this.mediaSource.endOfStream();
      } catch (error) {
        console.warn('MSE: Error ending stream:', error);
      }
    }

    this.audio.src = '';
    this.isPlaying = false;
  }

  public getBufferedSeconds(): number {
    if (!this.sourceBuffer || this.sourceBuffer.buffered.length === 0) {
      return 0;
    }
    const bufferedEnd = this.sourceBuffer.buffered.end(0);
    return bufferedEnd - this.audio.currentTime;
  }
}

// Helper function to detect MIME type from stream URL or headers
export function detectMimeType(contentType: string | null): string {
  if (!contentType) return 'audio/mpeg'; // Default to MP3

  if (contentType.includes('mp3') || contentType.includes('mpeg')) {
    return 'audio/mpeg';
  } else if (contentType.includes('opus') || contentType.includes('ogg')) {
    return 'audio/webm; codecs="opus"';
  } else if (contentType.includes('aac') || contentType.includes('mp4')) {
    return 'audio/mp4; codecs="mp4a.40.2"';
  }

  return 'audio/mpeg'; // Default fallback
}
