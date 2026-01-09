import { useAuthStore } from '../stores/authStore';

const API_BASE_URL = '/api';

// Helper to convert base64 to hex
export function base64ToHex(base64: string): string {
  const raw = atob(base64);
  let hex = '';
  for (let i = 0; i < raw.length; i++) {
    const hexByte = raw.charCodeAt(i).toString(16);
    hex += hexByte.padStart(2, '0');
  }
  return hex;
}

export class APIError extends Error {
  constructor(
    message: string,
    public status: number,
    public data?: unknown
  ) {
    super(message);
    this.name = 'APIError';
  }
}

interface RequestOptions extends Omit<RequestInit, 'headers'> {
  requiresAuth?: boolean;
  optionalAuth?: boolean; // Add auth headers if user is logged in, but don't throw error if not
  csrfToken?: string;
  headers?: Record<string, string>;
}

/**
 * Makes an authenticated API request with Ed25519 signature
 */
async function apiRequest<T>(
  endpoint: string,
  options: RequestOptions = {}
): Promise<T> {
  const { requiresAuth = false, optionalAuth = false, csrfToken, ...fetchOptions } = options;

  const url = `${API_BASE_URL}${endpoint}`;
  const timestamp = Math.floor(Date.now() / 1000).toString();

  const headers: Record<string, string> = {
    'Content-Type': 'application/json',
    ...(fetchOptions.headers as Record<string, string> || {}),
  };

  // Add authentication headers if required
  if (requiresAuth || optionalAuth) {
    const authState = useAuthStore.getState();

    // For requiresAuth, throw error if not authenticated
    // For optionalAuth, just skip adding headers if not authenticated
    if (!authState.isAuthenticated) {
      if (requiresAuth) {
        throw new APIError('Not authenticated', 401);
      }
      // For optionalAuth, continue without auth headers
    } else {
      // User is authenticated - handle both Ed25519 and magic link
      if (authState.authMethod === 'ed25519' && authState.publicKey) {
        // Ed25519 authentication: add signature headers
        const method = fetchOptions.method || 'GET';
        const path = new URL(url, window.location.origin).pathname;
        const message = `${method}${path}${timestamp}`;

        try {
          const signatureBase64 = authState.signMessage(message);

          // Convert base64 to hex for backend
          const signatureHex = base64ToHex(signatureBase64);
          const pubkeyHex = base64ToHex(authState.publicKey);

          headers['X-Signature'] = signatureHex;
          headers['X-Pubkey'] = pubkeyHex;
          headers['X-Timestamp'] = timestamp;
        } catch (error) {
          console.error('Failed to sign request:', error);
          if (requiresAuth) {
            throw new APIError('Failed to sign request', 500);
          }
          // For optionalAuth, continue without auth headers on error
        }
      }
      // For magic link users: cookie is sent automatically via credentials: 'include'
      // No additional headers needed
    }
  }

  // Add CSRF token for state-changing requests
  const usedCsrfToken = csrfToken && ['POST', 'PUT', 'PATCH', 'DELETE'].includes(fetchOptions.method || 'GET');
  if (usedCsrfToken) {
    headers['X-CSRF-Token'] = csrfToken;
  }

  try {
    const response = await fetch(url, {
      ...fetchOptions,
      headers,
      credentials: 'include', // Always send cookies (for magic link auth)
    });

    // IMPORTANT: Reset CSRF token cache after successful use
    // CSRF tokens are one-time use, so we must fetch a new one next time
    if (usedCsrfToken && response.ok) {
      resetCSRFToken();
    }

    // Handle non-JSON responses
    const contentType = response.headers.get('content-type');
    const isJson = contentType?.includes('application/json');

    if (!response.ok) {
      const errorData = isJson ? await response.json() : await response.text();
      throw new APIError(
        errorData?.message || `Request failed with status ${response.status}`,
        response.status,
        errorData
      );
    }

    // Handle empty responses
    if (response.status === 204 || !contentType) {
      return null as T;
    }

    if (!isJson) {
      return (await response.text()) as T;
    }

    return await response.json();
  } catch (error) {
    if (error instanceof APIError) {
      throw error;
    }

    // Network errors
    throw new APIError(
      error instanceof Error ? error.message : 'Network request failed',
      0
    );
  }
}

export const api = {
  get: <T>(endpoint: string, requiresAuth = false, optionalAuth = false) =>
    apiRequest<T>(endpoint, { method: 'GET', requiresAuth, optionalAuth }),

  post: <T>(endpoint: string, data?: unknown, requiresAuth = false, csrfToken?: string) =>
    apiRequest<T>(endpoint, {
      method: 'POST',
      body: JSON.stringify(data),
      requiresAuth,
      ...(csrfToken ? { csrfToken } : {}),
    }),

  put: <T>(endpoint: string, data?: unknown, requiresAuth = false, csrfToken?: string) =>
    apiRequest<T>(endpoint, {
      method: 'PUT',
      body: JSON.stringify(data),
      requiresAuth,
      ...(csrfToken ? { csrfToken } : {}),
    }),

  delete: <T>(endpoint: string, requiresAuth = false, csrfToken?: string) =>
    apiRequest<T>(endpoint, {
      method: 'DELETE',
      requiresAuth,
      ...(csrfToken ? { csrfToken } : {}),
    }),

  // File upload with multipart/form-data
  upload: async <T>(
    endpoint: string,
    formData: FormData,
    requiresAuth = true,
    csrfToken?: string
  ): Promise<T> => {
    const authState = useAuthStore.getState();
    const url = `${API_BASE_URL}${endpoint}`;
    const timestamp = Math.floor(Date.now() / 1000).toString();

    const headers: HeadersInit = {};

    if (requiresAuth) {
      if (!authState.isAuthenticated) {
        throw new APIError('Not authenticated', 401);
      }

      // Only add Ed25519 signature headers if using Ed25519 auth
      if (authState.authMethod === 'ed25519' && authState.publicKey) {
        const path = new URL(url, window.location.origin).pathname;
        const message = `POST${path}${timestamp}`;

        try {
          const signatureBase64 = authState.signMessage(message);

          // Convert base64 to hex for backend
          const signatureHex = base64ToHex(signatureBase64);
          const pubkeyHex = base64ToHex(authState.publicKey);

          headers['X-Signature'] = signatureHex;
          headers['X-Pubkey'] = pubkeyHex;
          headers['X-Timestamp'] = timestamp;
        } catch (error) {
          throw new APIError('Failed to sign request', 500);
        }
      }
      // For magic link: cookie sent via credentials: 'include'
    }

    if (csrfToken) {
      headers['X-CSRF-Token'] = csrfToken;
    }

    try {
      const response = await fetch(url, {
        method: 'POST',
        headers,
        body: formData,
        credentials: 'include', // Always send cookies
      });

      if (!response.ok) {
        const errorData = await response.json().catch(() => ({}));
        throw new APIError(
          errorData.message || `Upload failed with status ${response.status}`,
          response.status,
          errorData
        );
      }

      return await response.json();
    } catch (error) {
      if (error instanceof APIError) {
        throw error;
      }
      throw new APIError(
        error instanceof Error ? error.message : 'Upload failed',
        0
      );
    }
  },

  // Rating API functions

  // Rate a station (requires auth + CSRF)
  rateStation: async <T>(
    stationId: number,
    rating: number,
    csrfToken: string
  ): Promise<T> => {
    return apiRequest<T>(`/stations/${stationId}/rate`, {
      method: 'POST',
      body: JSON.stringify({ rating }),
      requiresAuth: true,
      csrfToken,
    });
  },

  // Rate a federated station by UUID (requires auth + CSRF)
  rateFederatedStation: async <T>(
    stationUUID: string,
    rating: number,
    csrfToken: string
  ): Promise<T> => {
    return apiRequest<T>(`/federated/stations/${stationUUID}/rate`, {
      method: 'POST',
      body: JSON.stringify({ rating }),
      requiresAuth: true,
      csrfToken,
    });
  },

  // Get rating stats (public)
  getRatingStats: async <T>(stationId: number): Promise<T> => {
    return apiRequest<T>(`/stations/${stationId}/ratings`, {
      method: 'GET',
    });
  },

  // Get current user's rating (requires auth)
  getMyRating: async <T>(stationId: number): Promise<T> => {
    return apiRequest<T>(`/stations/${stationId}/my-rating`, {
      method: 'GET',
      requiresAuth: true,
    });
  },
};

// CSRF token management
let csrfToken: string | null = null;

export async function fetchCSRFToken(): Promise<string> {
  if (csrfToken) {
    return csrfToken;
  }

  const response = await api.get<{ csrf_token: string }>('/csrf-token', true); // requires auth
  csrfToken = response.csrf_token;
  return csrfToken;
}

export function resetCSRFToken(): void {
  csrfToken = null;
}

// Station streaming credentials
export interface StreamingCredentials {
  username: string;
  password: string;
  mountpoint: string;
  stream_url: string;
  method: string;
}

export async function getStreamingCredentials(stationId: number): Promise<StreamingCredentials> {
  return api.get<StreamingCredentials>(`/stations/${stationId}/credentials`, true);
}

// Server info
export interface ServerInfo {
  name: string;
  description: string;
  version: string;
  port: number;
  features: {
    streaming: boolean;
    federation: boolean;
  };
  pow_difficulty: {
    comment: number;
    account: number;
    station: number;
  };
}

let cachedServerInfo: ServerInfo | null = null;

export async function getServerInfo(): Promise<ServerInfo> {
  if (cachedServerInfo) {
    return cachedServerInfo;
  }
  cachedServerInfo = await api.get<ServerInfo>('/info', false);
  return cachedServerInfo;
}

// Get PoW difficulty for specific action type
export async function getPoWDifficulty(action: 'comment' | 'account' | 'station'): Promise<number> {
  const info = await getServerInfo();
  return info.pow_difficulty[action];
}

// Playlist configuration
export interface PlaylistConfig {
  playlist_enabled: boolean;
  playlist_directory: string;
  playlist_mode: 'sequential' | 'shuffle';
  playlist_loop: boolean;
  playlist_file_pattern: string;
}

export async function updatePlaylistConfig(
  stationId: number,
  config: PlaylistConfig,
  csrfToken: string
): Promise<{ success: boolean }> {
  return api.put<{ success: boolean }>(
    `/stations/${stationId}/playlist-config`,
    config,
    true,
    csrfToken
  );
}

export async function downloadPlaylistScript(
  stationId: number,
  scriptType: 'bash' | 'powershell' | 'systemd'
): Promise<Blob> {
  const authState = useAuthStore.getState();

  if (!authState.isAuthenticated) {
    throw new APIError('Not authenticated', 401);
  }

  const url = `${API_BASE_URL}/stations/${stationId}/playlist-script?type=${scriptType}`;
  const timestamp = Math.floor(Date.now() / 1000).toString();
  const path = new URL(url, window.location.origin).pathname + new URL(url, window.location.origin).search;

  const headers: HeadersInit = {};

  // Only add Ed25519 signature headers if using Ed25519 auth
  if (authState.authMethod === 'ed25519' && authState.publicKey) {
    const message = `GET${path}${timestamp}`;

    try {
      const signatureBase64 = authState.signMessage(message);
      const signatureHex = base64ToHex(signatureBase64);
      const pubkeyHex = base64ToHex(authState.publicKey);

      headers['X-Signature'] = signatureHex;
      headers['X-Pubkey'] = pubkeyHex;
      headers['X-Timestamp'] = timestamp;
    } catch (error) {
      throw new APIError('Failed to sign request', 500);
    }
  }
  // For magic link: cookie sent via credentials: 'include'

  try {
    const response = await fetch(url, {
      method: 'GET',
      headers,
      credentials: 'include', // Always send cookies
    });

    if (!response.ok) {
      const errorData = await response.json().catch(() => ({}));
      throw new APIError(
        errorData.message || `Download failed with status ${response.status}`,
        response.status,
        errorData
      );
    }

    return await response.blob();
  } catch (error) {
    if (error instanceof APIError) {
      throw error;
    }
    throw new APIError(
      error instanceof Error ? error.message : 'Download failed',
      0
    );
  }
}
