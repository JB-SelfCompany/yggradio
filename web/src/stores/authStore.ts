import { create } from 'zustand';
import { persist, createJSONStorage } from 'zustand/middleware';
import nacl from 'tweetnacl';
import { encodeBase64, decodeBase64 } from 'tweetnacl-util';

interface AuthState {
  publicKey: string | null;
  privateKey: string | null;
  isAuthenticated: boolean;
  isAdmin: boolean;

  // NEW: Auth method tracking
  authMethod: 'ed25519' | 'magic_link' | null;
  userId: number | null;

  // Actions
  generateKeyPair: () => void;
  importKeyPair: (publicKey: string, privateKey: string) => void;
  logout: () => Promise<void>;
  signMessage: (message: string) => string;
  setIsAdmin: (isAdmin: boolean) => void;

  // NEW: Magic link actions
  loginWithMagicLink: () => Promise<void>;
  setAuthMethod: (method: 'ed25519' | 'magic_link', userId?: number) => void;
}

export const useAuthStore = create<AuthState>()(
  persist(
    (set, get) => ({
      publicKey: null,
      privateKey: null,
      isAuthenticated: false,
      isAdmin: false,
      authMethod: null,
      userId: null,

      generateKeyPair: () => {
        const keyPair = nacl.sign.keyPair();
        const publicKey = encodeBase64(keyPair.publicKey);
        const privateKey = encodeBase64(keyPair.secretKey);

        set({
          publicKey,
          privateKey,
          isAuthenticated: true,
          authMethod: 'ed25519',
          userId: null, // Will be set after first API call
        });
      },

      importKeyPair: (publicKey: string, privateKey: string) => {
        try {
          // Validate that keys are valid base64
          decodeBase64(publicKey);
          decodeBase64(privateKey);

          set({
            publicKey,
            privateKey,
            isAuthenticated: true,
            authMethod: 'ed25519',
            userId: null, // Will be set after first API call
          });
        } catch (error) {
          console.error('Invalid key format:', error);
          throw new Error('Invalid key format');
        }
      },

      logout: async () => {
        const state = get();

        // Call logout API if authenticated
        if (state.isAuthenticated) {
          try {
            await fetch('/api/auth/logout', {
              method: 'POST',
              credentials: 'include', // Important for cookies
              headers: {
                'Content-Type': 'application/json',
              },
            });
          } catch (error) {
            console.error('Logout API call failed:', error);
            // Continue with local logout even if API call fails
          }
        }

        set({
          publicKey: null,
          privateKey: null,
          isAuthenticated: false,
          isAdmin: false,
          authMethod: null,
          userId: null,
        });
      },

      signMessage: (message: string) => {
        const state = get();
        if (!state.privateKey) {
          throw new Error('No private key available');
        }

        try {
          const privateKeyBytes = decodeBase64(state.privateKey);
          const messageBytes = new TextEncoder().encode(message);
          const signature = nacl.sign.detached(messageBytes, privateKeyBytes);
          return encodeBase64(signature);
        } catch (error) {
          console.error('Signing failed:', error);
          throw new Error('Failed to sign message');
        }
      },

      setIsAdmin: (isAdmin: boolean) => {
        set({ isAdmin });
      },

      // NEW: Login with magic link (cookie-based auth)
      loginWithMagicLink: async () => {
        try {
          // Cookie is already set by the /login?key=<token> endpoint
          // We just need to verify it works by fetching user profile
          const response = await fetch('/api/user/profile', {
            credentials: 'include', // Important: send cookies
          });

          if (response.ok) {
            const data = await response.json();
            set({
              authMethod: 'magic_link',
              userId: data.user_id,
              isAuthenticated: true,
              isAdmin: data.is_admin || false,
              publicKey: null,
              privateKey: null,
            });
          } else {
            throw new Error('Failed to verify magic link session');
          }
        } catch (error) {
          console.error('Magic link login failed:', error);
          throw error;
        }
      },

      // NEW: Set auth method (used by API after successful auth)
      setAuthMethod: (method: 'ed25519' | 'magic_link', userId?: number) => {
        set({
          authMethod: method,
          userId: userId || null,
        });
      },
    }),
    {
      name: 'yggradio-auth',
      // CHANGED: Use sessionStorage instead of localStorage for better security
      // Keys are cleared when browser is closed
      storage: createJSONStorage(() => sessionStorage),
      partialize: (state) => ({
        publicKey: state.publicKey,
        privateKey: state.privateKey,
        isAuthenticated: state.isAuthenticated,
        isAdmin: state.isAdmin,
        authMethod: state.authMethod, // NEW: persist auth method
        userId: state.userId, // NEW: persist user ID
      }),
      // Validate state on rehydration
      onRehydrateStorage: () => (state) => {
        if (!state) return;

        // For Ed25519 users: if private key is missing, force logout
        if (state.authMethod === 'ed25519' && state.isAuthenticated && !state.privateKey) {
          state.publicKey = null;
          state.privateKey = null;
          state.isAuthenticated = false;
          state.isAdmin = false;
          state.authMethod = null;
          state.userId = null;
        }

        // For magic link users: validate session by attempting to fetch profile
        if (state.authMethod === 'magic_link' && state.isAuthenticated) {
          // Validate cookie is still valid
          fetch('/api/user/profile', { credentials: 'include' })
            .then((response) => {
              if (!response.ok) {
                // Cookie expired or invalid, force logout
                state.publicKey = null;
                state.privateKey = null;
                state.isAuthenticated = false;
                state.isAdmin = false;
                state.authMethod = null;
                state.userId = null;
              }
            })
            .catch(() => {
              // Network error, don't force logout
              // Cookie might still be valid
            });
        }
      },
    }
  )
);
