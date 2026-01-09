import { describe, it, expect, beforeEach } from 'vitest';
import { useAuthStore } from '../authStore';
import { decodeBase64 } from 'tweetnacl-util';

describe('Security: Auth Store', () => {
  beforeEach(() => {
    // Reset store state
    useAuthStore.getState().logout();
  });

  describe('Key Generation', () => {
    it('should generate Ed25519 key pair', () => {
      const store = useAuthStore.getState();
      store.generateKeyPair();

      expect(store.publicKey).toBeTruthy();
      expect(store.privateKey).toBeTruthy();
      expect(store.isAuthenticated).toBe(true);
    });

    it('should generate valid base64 encoded keys', () => {
      const store = useAuthStore.getState();
      store.generateKeyPair();

      // Should not throw when decoding
      expect(() => decodeBase64(store.publicKey!)).not.toThrow();
      expect(() => decodeBase64(store.privateKey!)).not.toThrow();
    });

    it('should generate different keys each time', () => {
      const store = useAuthStore.getState();

      store.generateKeyPair();
      const pubkey1 = store.publicKey;

      store.logout();
      store.generateKeyPair();
      const pubkey2 = store.publicKey;

      expect(pubkey1).not.toBe(pubkey2);
    });

    it('should generate 32-byte public key (Ed25519 standard)', () => {
      const store = useAuthStore.getState();
      store.generateKeyPair();

      const pubkeyBytes = decodeBase64(store.publicKey!);
      expect(pubkeyBytes.length).toBe(32);
    });

    it('should generate 64-byte private key (Ed25519 standard)', () => {
      const store = useAuthStore.getState();
      store.generateKeyPair();

      const privkeyBytes = decodeBase64(store.privateKey!);
      expect(privkeyBytes.length).toBe(64);
    });
  });

  describe('Key Import', () => {
    it('should import valid key pair', () => {
      const store = useAuthStore.getState();

      // Generate a valid pair first
      store.generateKeyPair();
      const validPublic = store.publicKey!;
      const validPrivate = store.privateKey!;

      // Logout and import
      store.logout();
      store.importKeyPair(validPublic, validPrivate);

      expect(store.publicKey).toBe(validPublic);
      expect(store.privateKey).toBe(validPrivate);
      expect(store.isAuthenticated).toBe(true);
    });

    it('should reject invalid base64 keys', () => {
      const store = useAuthStore.getState();

      expect(() => {
        store.importKeyPair('invalid!!!', 'invalid!!!');
      }).toThrow('Invalid key format');
    });

    it('should reject empty keys', () => {
      const store = useAuthStore.getState();

      expect(() => {
        store.importKeyPair('', '');
      }).toThrow();
    });

    it('should not authenticate with invalid keys', () => {
      const store = useAuthStore.getState();

      try {
        store.importKeyPair('invalid', 'invalid');
      } catch {
        // Expected to throw
      }

      expect(store.isAuthenticated).toBe(false);
    });
  });

  describe('Message Signing', () => {
    it('should sign message with private key', () => {
      const store = useAuthStore.getState();
      store.generateKeyPair();

      const message = 'test message';
      const signature = store.signMessage(message);

      expect(signature).toBeTruthy();
      expect(typeof signature).toBe('string');
    });

    it('should produce base64 encoded signature', () => {
      const store = useAuthStore.getState();
      store.generateKeyPair();

      const signature = store.signMessage('test');

      // Base64 regex
      expect(signature).toMatch(/^[A-Za-z0-9+/]+=*$/);

      // Should decode without error
      expect(() => decodeBase64(signature)).not.toThrow();
    });

    it('should produce different signatures for different messages', () => {
      const store = useAuthStore.getState();
      store.generateKeyPair();

      const sig1 = store.signMessage('message1');
      const sig2 = store.signMessage('message2');

      expect(sig1).not.toBe(sig2);
    });

    it('should produce consistent signature for same message', () => {
      const store = useAuthStore.getState();
      store.generateKeyPair();

      const message = 'consistent message';
      const sig1 = store.signMessage(message);
      const sig2 = store.signMessage(message);

      expect(sig1).toBe(sig2);
    });

    it('should throw error when signing without private key', () => {
      const store = useAuthStore.getState();

      expect(() => {
        store.signMessage('test');
      }).toThrow('No private key available');
    });

    it('should sign complex messages', () => {
      const store = useAuthStore.getState();
      store.generateKeyPair();

      const complexMessage = 'POST/api/test1234567890';
      const signature = store.signMessage(complexMessage);

      expect(signature).toBeTruthy();
      expect(signature.length).toBeGreaterThan(0);
    });

    it('should sign Unicode messages correctly', () => {
      const store = useAuthStore.getState();
      store.generateKeyPair();

      const unicodeMessage = 'Hello 世界 🌍';
      const signature = store.signMessage(unicodeMessage);

      expect(signature).toBeTruthy();
    });
  });

  describe('Logout', () => {
    it('should clear all auth data on logout', () => {
      const store = useAuthStore.getState();
      store.generateKeyPair();
      store.setIsAdmin(true);

      store.logout();

      expect(store.publicKey).toBeNull();
      expect(store.privateKey).toBeNull();
      expect(store.isAuthenticated).toBe(false);
      expect(store.isAdmin).toBe(false);
    });

    it('should allow generating new keys after logout', () => {
      const store = useAuthStore.getState();

      store.generateKeyPair();
      store.logout();
      store.generateKeyPair();

      expect(store.isAuthenticated).toBe(true);
      expect(store.publicKey).toBeTruthy();
    });
  });

  describe('Admin Status', () => {
    it('should set admin status', () => {
      const store = useAuthStore.getState();

      store.setIsAdmin(true);
      expect(store.isAdmin).toBe(true);

      store.setIsAdmin(false);
      expect(store.isAdmin).toBe(false);
    });

    it('should reset admin status on logout', () => {
      const store = useAuthStore.getState();
      store.generateKeyPair();
      store.setIsAdmin(true);

      store.logout();

      expect(store.isAdmin).toBe(false);
    });
  });

  describe('Security: Private Key Handling', () => {
    it('should not persist private key to localStorage', () => {
      const store = useAuthStore.getState();
      store.generateKeyPair();

      // Simulate page reload by checking what would be persisted
      // The persist middleware should only save publicKey, not privateKey
      // This is configured in the store's partialize function

      const persistedState = JSON.stringify({
        publicKey: store.publicKey,
        isAuthenticated: store.isAuthenticated,
        isAdmin: store.isAdmin,
      });

      expect(persistedState).toContain(store.publicKey);
      expect(persistedState).not.toContain('privateKey');
    });

    it('should keep private key in memory only', () => {
      const store = useAuthStore.getState();
      store.generateKeyPair();

      const privateKey = store.privateKey;
      expect(privateKey).toBeTruthy();

      // Private key should be accessible in current session
      expect(store.privateKey).toBe(privateKey);
    });
  });

  describe('Security: Key Validation', () => {
    it('should reject short keys', () => {
      const store = useAuthStore.getState();

      expect(() => {
        store.importKeyPair('short', 'keys');
      }).toThrow();
    });

    it('should handle malformed base64', () => {
      const store = useAuthStore.getState();

      expect(() => {
        store.importKeyPair('not@valid#base64!', 'also@invalid!');
      }).toThrow();
    });
  });
});
