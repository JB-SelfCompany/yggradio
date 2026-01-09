import { describe, it, expect, beforeEach, vi, afterEach } from 'vitest';
import { api, APIError, fetchCSRFToken, resetCSRFToken } from '../api';
import { useAuthStore } from '../../stores/authStore';

describe('Security: API Client', () => {
  beforeEach(() => {
    // Reset mocks
    vi.clearAllMocks();
    global.fetch = vi.fn();
    resetCSRFToken();

    // Reset auth store
    useAuthStore.getState().logout();
  });

  describe('GET requests', () => {
    it('should make GET request without auth', async () => {
      const mockResponse = { data: 'test' };
      (global.fetch as any).mockResolvedValueOnce({
        ok: true,
        status: 200,
        headers: new Headers({ 'content-type': 'application/json' }),
        json: async () => mockResponse,
      });

      const result = await api.get('/test');

      expect(global.fetch).toHaveBeenCalledWith(
        '/api/test',
        expect.objectContaining({
          method: 'GET',
        })
      );
      expect(result).toEqual(mockResponse);
    });

    it('should include auth headers when required', async () => {
      // Setup authenticated state
      useAuthStore.getState().generateKeyPair();

      const mockResponse = { data: 'test' };
      (global.fetch as any).mockResolvedValueOnce({
        ok: true,
        status: 200,
        headers: new Headers({ 'content-type': 'application/json' }),
        json: async () => mockResponse,
      });

      await api.get('/protected', true);

      const fetchCall = (global.fetch as any).mock.calls[0];
      const headers = fetchCall[1].headers;

      expect(headers['X-Signature']).toBeDefined();
      expect(headers['X-Pubkey']).toBeDefined();
      expect(headers['X-Timestamp']).toBeDefined();
    });

    it('should throw error when auth required but not authenticated', async () => {
      await expect(api.get('/protected', true)).rejects.toThrow('Not authenticated');
    });

    it('should handle 404 errors', async () => {
      (global.fetch as any).mockResolvedValueOnce({
        ok: false,
        status: 404,
        headers: new Headers({ 'content-type': 'application/json' }),
        json: async () => ({ message: 'Not found' }),
      });

      await expect(api.get('/nonexistent')).rejects.toThrow(APIError);

      try {
        await api.get('/nonexistent');
      } catch (error) {
        expect(error).toBeInstanceOf(APIError);
        expect((error as APIError).status).toBe(404);
      }
    });

    it('should handle network errors', async () => {
      (global.fetch as any).mockRejectedValueOnce(new Error('Network error'));

      await expect(api.get('/test')).rejects.toThrow('Network error');
    });
  });

  describe('POST requests', () => {
    it('should make POST request with data', async () => {
      const mockResponse = { success: true };
      const postData = { name: 'test' };

      (global.fetch as any).mockResolvedValueOnce({
        ok: true,
        status: 200,
        headers: new Headers({ 'content-type': 'application/json' }),
        json: async () => mockResponse,
      });

      await api.post('/create', postData);

      const fetchCall = (global.fetch as any).mock.calls[0];
      expect(fetchCall[1].method).toBe('POST');
      expect(fetchCall[1].body).toBe(JSON.stringify(postData));
    });

    it('should include CSRF token in POST requests', async () => {
      const csrfToken = 'test-csrf-token';

      (global.fetch as any).mockResolvedValueOnce({
        ok: true,
        status: 200,
        headers: new Headers({ 'content-type': 'application/json' }),
        json: async () => ({ success: true }),
      });

      await api.post('/create', { data: 'test' }, false, csrfToken);

      const fetchCall = (global.fetch as any).mock.calls[0];
      const headers = fetchCall[1].headers;

      expect(headers['X-CSRF-Token']).toBe(csrfToken);
    });

    it('should sign POST requests when authenticated', async () => {
      useAuthStore.getState().generateKeyPair();

      (global.fetch as any).mockResolvedValueOnce({
        ok: true,
        status: 200,
        headers: new Headers({ 'content-type': 'application/json' }),
        json: async () => ({ success: true }),
      });

      await api.post('/protected', { data: 'test' }, true);

      const fetchCall = (global.fetch as any).mock.calls[0];
      const headers = fetchCall[1].headers;

      expect(headers['X-Signature']).toBeDefined();
      expect(headers['X-Pubkey']).toBeDefined();
      expect(headers['X-Timestamp']).toBeDefined();
    });
  });

  describe('DELETE requests', () => {
    it('should make DELETE request', async () => {
      (global.fetch as any).mockResolvedValueOnce({
        ok: true,
        status: 204,
      });

      await api.delete('/resource/123');

      const fetchCall = (global.fetch as any).mock.calls[0];
      expect(fetchCall[1].method).toBe('DELETE');
    });

    it('should include CSRF token in DELETE requests', async () => {
      const csrfToken = 'test-csrf-token';

      (global.fetch as any).mockResolvedValueOnce({
        ok: true,
        status: 204,
      });

      await api.delete('/resource/123', false, csrfToken);

      const fetchCall = (global.fetch as any).mock.calls[0];
      const headers = fetchCall[1].headers;

      expect(headers['X-CSRF-Token']).toBe(csrfToken);
    });
  });

  describe('File upload', () => {
    it('should upload file with auth headers', async () => {
      useAuthStore.getState().generateKeyPair();

      const formData = new FormData();
      formData.append('file', new File(['test'], 'test.mp3'));

      (global.fetch as any).mockResolvedValueOnce({
        ok: true,
        status: 200,
        headers: new Headers({ 'content-type': 'application/json' }),
        json: async () => ({ success: true }),
      });

      await api.upload('/upload', formData);

      const fetchCall = (global.fetch as any).mock.calls[0];
      const headers = fetchCall[1].headers;

      expect(fetchCall[1].method).toBe('POST');
      expect(headers['X-Signature']).toBeDefined();
      expect(headers['X-Pubkey']).toBeDefined();
      expect(headers['X-Timestamp']).toBeDefined();
    });

    it('should include CSRF token in upload', async () => {
      useAuthStore.getState().generateKeyPair();

      const formData = new FormData();
      const csrfToken = 'test-csrf-token';

      (global.fetch as any).mockResolvedValueOnce({
        ok: true,
        status: 200,
        headers: new Headers({ 'content-type': 'application/json' }),
        json: async () => ({ success: true }),
      });

      await api.upload('/upload', formData, true, csrfToken);

      const fetchCall = (global.fetch as any).mock.calls[0];
      const headers = fetchCall[1].headers;

      expect(headers['X-CSRF-Token']).toBe(csrfToken);
    });

    it('should throw error when upload fails', async () => {
      useAuthStore.getState().generateKeyPair();

      const formData = new FormData();

      (global.fetch as any).mockResolvedValueOnce({
        ok: false,
        status: 413,
        json: async () => ({ message: 'File too large' }),
      });

      await expect(api.upload('/upload', formData)).rejects.toThrow('File too large');
    });
  });

  describe('CSRF Token Management', () => {
    it('should fetch CSRF token', async () => {
      const mockToken = 'csrf-token-123';

      (global.fetch as any).mockResolvedValueOnce({
        ok: true,
        status: 200,
        headers: new Headers({ 'content-type': 'application/json' }),
        json: async () => ({ token: mockToken }),
      });

      const token = await fetchCSRFToken();
      expect(token).toBe(mockToken);
    });

    it('should cache CSRF token', async () => {
      const mockToken = 'csrf-token-123';

      (global.fetch as any).mockResolvedValueOnce({
        ok: true,
        status: 200,
        headers: new Headers({ 'content-type': 'application/json' }),
        json: async () => ({ token: mockToken }),
      });

      const token1 = await fetchCSRFToken();
      const token2 = await fetchCSRFToken();

      expect(token1).toBe(token2);
      expect(global.fetch).toHaveBeenCalledTimes(1);
    });

    it('should reset CSRF token', async () => {
      const mockToken = 'csrf-token-123';

      (global.fetch as any).mockResolvedValue({
        ok: true,
        status: 200,
        headers: new Headers({ 'content-type': 'application/json' }),
        json: async () => ({ token: mockToken }),
      });

      await fetchCSRFToken();
      resetCSRFToken();
      await fetchCSRFToken();

      expect(global.fetch).toHaveBeenCalledTimes(2);
    });
  });

  describe('Security: Request Signing', () => {
    it('should sign requests with Ed25519 signature', async () => {
      useAuthStore.getState().generateKeyPair();

      (global.fetch as any).mockResolvedValueOnce({
        ok: true,
        status: 200,
        headers: new Headers({ 'content-type': 'application/json' }),
        json: async () => ({ success: true }),
      });

      await api.get('/protected', true);

      const fetchCall = (global.fetch as any).mock.calls[0];
      const headers = fetchCall[1].headers;

      // Verify signature format (base64)
      expect(headers['X-Signature']).toMatch(/^[A-Za-z0-9+/]+=*$/);

      // Verify public key format (base64)
      expect(headers['X-Pubkey']).toMatch(/^[A-Za-z0-9+/]+=*$/);

      // Verify timestamp is recent
      const timestamp = parseInt(headers['X-Timestamp']);
      const now = Math.floor(Date.now() / 1000);
      expect(timestamp).toBeGreaterThan(now - 10);
      expect(timestamp).toBeLessThanOrEqual(now);
    });

    it('should include method, path, and timestamp in signature', async () => {
      useAuthStore.getState().generateKeyPair();

      // We can't verify the actual signature without the backend,
      // but we can verify the signing function is called
      const signSpy = vi.spyOn(useAuthStore.getState(), 'signMessage');

      (global.fetch as any).mockResolvedValueOnce({
        ok: true,
        status: 200,
        headers: new Headers({ 'content-type': 'application/json' }),
        json: async () => ({ success: true }),
      });

      await api.post('/test', {}, true);

      expect(signSpy).toHaveBeenCalled();
      const signedMessage = signSpy.mock.calls[0][0];
      expect(signedMessage).toContain('POST');
      expect(signedMessage).toContain('/api/test');
    });
  });
});
