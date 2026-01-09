/**
 * Proof-of-Work implementation for anti-spam protection
 * Implements Hashcash-style PoW algorithm
 */

import { sha256 as jsSha256 } from 'js-sha256';

// Helper to convert base64 to hex
function base64ToHex(base64: string): string {
  const raw = atob(base64);
  let hex = '';
  for (let i = 0; i < raw.length; i++) {
    const hexByte = raw.charCodeAt(i).toString(16);
    hex += hexByte.padStart(2, '0');
  }
  return hex;
}

export { base64ToHex };

/**
 * Calculate SHA-256 hash
 * Uses crypto.subtle if available (HTTPS), otherwise falls back to js-sha256
 */
async function sha256(message: string): Promise<string> {
  // Check if crypto.subtle is available (requires secure context)
  if (typeof crypto !== 'undefined' && crypto.subtle) {
    try {
      const encoder = new TextEncoder();
      const data = encoder.encode(message);
      const hashBuffer = await crypto.subtle.digest('SHA-256', data);
      const hashArray = Array.from(new Uint8Array(hashBuffer));
      const hashHex = hashArray.map((b) => b.toString(16).padStart(2, '0')).join('');
      return hashHex;
    } catch (e) {
      // Fallback to js-sha256 if crypto.subtle fails
      console.warn('crypto.subtle failed, using js-sha256 fallback:', e);
    }
  }

  // Fallback for non-secure contexts (HTTP)
  return jsSha256(message);
}

/**
 * Count leading zero bits in a hex string
 */
function countLeadingZeroBits(hex: string): number {
  let count = 0;

  for (let i = 0; i < hex.length; i++) {
    const char = hex[i];
    if (!char) continue; // Safety check for TypeScript

    const nibble = parseInt(char, 16);

    if (nibble === 0) {
      count += 4;
    } else {
      // Count leading zeros in the nibble
      for (let bit = 3; bit >= 0; bit--) {
        if ((nibble >> bit) & 1) {
          return count;
        }
        count++;
      }
      return count;
    }
  }

  return count;
}

/**
 * Calculate Proof-of-Work nonce
 * @param pubkey - User's public key (hex string)
 * @param timestamp - Unix timestamp
 * @param difficulty - Number of leading zero bits required (4-20)
 * @returns Promise<string> - The nonce that satisfies the difficulty
 */
export async function calculatePoW(
  pubkey: string,
  timestamp: number,
  difficulty: number
): Promise<string> {
  let nonce = 0;
  const maxAttempts = 10000000; // 10 million attempts max

  while (nonce < maxAttempts) {
    const message = `${pubkey}${timestamp}${nonce}`;
    const hash = await sha256(message);
    const leadingZeros = countLeadingZeroBits(hash);

    if (leadingZeros >= difficulty) {
      return nonce.toString();
    }

    nonce++;

    // Update UI every 1000 iterations to prevent freezing
    if (nonce % 1000 === 0) {
      await new Promise((resolve) => setTimeout(resolve, 0));
    }
  }

  throw new Error('Failed to calculate Proof-of-Work: max attempts exceeded');
}

/**
 * Verify Proof-of-Work
 * @param pubkey - User's public key
 * @param timestamp - Unix timestamp
 * @param nonce - Nonce to verify
 * @param difficulty - Required difficulty
 * @returns Promise<boolean> - Whether the PoW is valid
 */
export async function verifyPoW(
  pubkey: string,
  timestamp: number,
  nonce: string,
  difficulty: number
): Promise<boolean> {
  const message = `${pubkey}${timestamp}${nonce}`;
  const hash = await sha256(message);
  const leadingZeros = countLeadingZeroBits(hash);
  return leadingZeros >= difficulty;
}

/**
 * Estimate time to calculate PoW (in milliseconds)
 * Based on average of ~1 million hashes per second on typical CPU
 */
export function estimatePoWTime(difficulty: number): number {
  const attempts = Math.pow(2, difficulty);
  const hashesPerSecond = 1000000;
  const seconds = attempts / hashesPerSecond;
  return seconds * 1000; // Convert to milliseconds
}

/**
 * Calculate Proof-of-Work for magic link registration
 * Uses a custom challenge string instead of pubkey+timestamp+nonce
 * @param challenge - Challenge string (e.g., "register_magic_link_1234567890")
 * @param difficulty - Number of leading zero bits required (4-20)
 * @returns Promise with hash and nonce
 */
export async function calculatePoWForChallenge(
  challenge: string,
  difficulty: number
): Promise<{ hash: string; nonce: number }> {
  let nonce = 0;
  const maxAttempts = 10000000; // 10 million attempts max

  while (nonce < maxAttempts) {
    const message = `${challenge}${nonce}`;
    const hash = await sha256(message);
    const leadingZeros = countLeadingZeroBits(hash);

    if (leadingZeros >= difficulty) {
      return { hash, nonce };
    }

    nonce++;

    // Update UI every 1000 iterations to prevent freezing
    if (nonce % 1000 === 0) {
      await new Promise((resolve) => setTimeout(resolve, 0));
    }
  }

  throw new Error('Failed to calculate Proof-of-Work: max attempts exceeded');
}
