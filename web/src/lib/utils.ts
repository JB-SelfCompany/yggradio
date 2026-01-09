/**
 * Utility functions for YggRadio
 */

/**
 * Checks if a URL points to a Yggdrasil stream
 * Yggdrasil addresses start with 0200-03ff range
 */
export function isYggdrasilStream(url: string): boolean {
  // Yggdrasil addresses start with 0200-03ff range
  const yggdrasilPattern = /\[0[23][0-9a-f]{2}:[0-9a-f:]+\]/i;
  return yggdrasilPattern.test(url);
}

/**
 * Formats a time duration in seconds to MM:SS format
 */
export function formatTime(seconds: number): string {
  if (!isFinite(seconds) || seconds < 0) return '0:00';

  const mins = Math.floor(seconds / 60);
  const secs = Math.floor(seconds % 60);
  return `${mins}:${secs.toString().padStart(2, '0')}`;
}

/**
 * Formats a byte size to human-readable format
 */
export function formatBytes(bytes: number): string {
  if (bytes === 0) return '0 Bytes';

  const k = 1024;
  const sizes = ['Bytes', 'KB', 'MB', 'GB'];
  const i = Math.floor(Math.log(bytes) / Math.log(k));

  return `${Math.round(bytes / Math.pow(k, i) * 100) / 100} ${sizes[i]}`;
}

/**
 * Debounces a function call
 */
export function debounce<T extends (...args: any[]) => any>(
  func: T,
  wait: number
): (...args: Parameters<T>) => void {
  let timeout: ReturnType<typeof setTimeout> | null = null;

  return (...args: Parameters<T>) => {
    if (timeout) clearTimeout(timeout);
    timeout = setTimeout(() => func(...args), wait);
  };
}
