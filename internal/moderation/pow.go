package moderation

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"
)

// ProofOfWork implements a Hashcash-style proof-of-work system
type ProofOfWork struct {
	difficulty int // Number of leading zero bits required (4-20)
}

// Challenge represents a PoW challenge
type Challenge struct {
	Pubkey     string `json:"pubkey"`
	Timestamp  int64  `json:"timestamp"`
	Difficulty int    `json:"difficulty"`
	Nonce      string `json:"nonce,omitempty"`
}

// NewProofOfWork creates a new PoW instance with specified difficulty
func NewProofOfWork(difficulty int) *ProofOfWork {
	// Validate difficulty (4-20 bits)
	if difficulty < 4 {
		difficulty = 4
	}
	if difficulty > 20 {
		difficulty = 20
	}

	return &ProofOfWork{
		difficulty: difficulty,
	}
}

// GenerateChallenge creates a new challenge for a client
func (p *ProofOfWork) GenerateChallenge(pubkey string) *Challenge {
	return &Challenge{
		Pubkey:     pubkey,
		Timestamp:  time.Now().Unix(),
		Difficulty: p.difficulty,
	}
}

// Verify validates a PoW solution
func (p *ProofOfWork) Verify(challenge *Challenge, nonce string) bool {
	// Check timestamp validity (protection against replay attacks)
	currentTime := time.Now().Unix()
	timeDiff := currentTime - challenge.Timestamp

	// Allow up to 15 minutes in the past (for slow PoW calculation), up to 2 minutes in the future
	// PoW calculation can take several minutes on mobile devices or with high difficulty
	// Future buffer accounts for clock skew between client and server
	maxPast := int64(905)    // 15 minutes + 5 seconds buffer
	maxFuture := int64(-125) // 2 minutes + 5 seconds buffer (negative because future)

	if timeDiff > maxPast || timeDiff < maxFuture {
		fmt.Printf("PoW timestamp check failed: currentTime=%d, timestamp=%d, diff=%d (maxPast=%d, maxFuture=%d)\n",
			currentTime, challenge.Timestamp, timeDiff, maxPast, maxFuture)
		return false
	}

	// Compute hash
	data := fmt.Sprintf("%s%d%s", challenge.Pubkey, challenge.Timestamp, nonce)
	hash := sha256.Sum256([]byte(data))

	// Check leading zero bits
	leadingZeros := countLeadingZeroBits(hash[:])

	if leadingZeros < p.difficulty {
		fmt.Printf("PoW difficulty check failed: data=%s, hash=%x, leadingZeros=%d, required=%d\n",
			data, hash, leadingZeros, p.difficulty)
		return false
	}

	fmt.Printf("PoW verified successfully: data=%s, leadingZeros=%d, required=%d\n",
		data, leadingZeros, p.difficulty)
	return true
}

// Solve solves a PoW challenge (for client use)
// WARNING: This is CPU-intensive and should run in a goroutine
func (p *ProofOfWork) Solve(challenge *Challenge) (string, error) {
	nonce := 0
	maxAttempts := 10000000 // Prevent infinite loops

	for nonce < maxAttempts {
		nonceStr := fmt.Sprintf("%d", nonce)

		if p.Verify(challenge, nonceStr) {
			return nonceStr, nil
		}

		nonce++
	}

	return "", fmt.Errorf("failed to solve PoW after %d attempts", maxAttempts)
}

// ComputeHash computes the hash for a challenge and nonce
func (p *ProofOfWork) ComputeHash(challenge *Challenge, nonce string) string {
	data := fmt.Sprintf("%s%d%s", challenge.Pubkey, challenge.Timestamp, nonce)
	hash := sha256.Sum256([]byte(data))
	return hex.EncodeToString(hash[:])
}

// countLeadingZeroBits counts the number of leading zero bits in a byte array
func countLeadingZeroBits(hash []byte) int {
	count := 0

	for _, b := range hash {
		if b == 0 {
			count += 8
		} else {
			// Count bits in the first non-zero byte
			for i := 7; i >= 0; i-- {
				if (b>>i)&1 == 0 {
					count++
				} else {
					return count
				}
			}
		}
	}

	return count
}

// EstimateTime estimates the time required to solve a PoW challenge
func (p *ProofOfWork) EstimateTime(difficulty int) time.Duration {
	// Average attempts needed = 2^difficulty
	attempts := 1 << difficulty

	// Assume ~1 million hashes per second on average CPU
	hashesPerSecond := 1000000

	seconds := float64(attempts) / float64(hashesPerSecond)

	return time.Duration(seconds * float64(time.Second))
}

// GetDifficulty returns the current difficulty level
func (p *ProofOfWork) GetDifficulty() int {
	return p.difficulty
}

// ValidateChallenge validates challenge parameters
func (p *ProofOfWork) ValidateChallenge(challenge *Challenge) error {
	if challenge.Pubkey == "" {
		return fmt.Errorf("missing pubkey")
	}

	if challenge.Timestamp == 0 {
		return fmt.Errorf("missing timestamp")
	}

	if challenge.Difficulty < 4 || challenge.Difficulty > 20 {
		return fmt.Errorf("invalid difficulty: %d (must be 4-20)", challenge.Difficulty)
	}

	// Check timestamp is within valid window
	currentTime := time.Now().Unix()
	timeDiff := currentTime - challenge.Timestamp

	// Same tolerance as Verify() to ensure consistency
	maxPast := int64(905)    // 15 minutes + 5 seconds buffer
	maxFuture := int64(-125) // 2 minutes + 5 seconds buffer

	if timeDiff > maxPast || timeDiff < maxFuture {
		return fmt.Errorf("timestamp outside valid window")
	}

	return nil
}

// VerifyHash verifies that a hash meets the difficulty requirement
func (p *ProofOfWork) VerifyHash(hash string) bool {
	// Decode hex hash
	hashBytes, err := hex.DecodeString(hash)
	if err != nil {
		return false
	}

	// Check hash length
	if len(hashBytes) != 32 {
		return false
	}

	// Count leading zero bits
	leadingZeros := countLeadingZeroBits(hashBytes)

	return leadingZeros >= p.difficulty
}

// GetAdaptiveDifficulty adjusts PoW difficulty based on recent creation activity
// SECURITY: Increases difficulty during high-volume periods to prevent spam attacks
// recentCreations: number of accounts/stations created in the last hour from this subnet
func (p *ProofOfWork) GetAdaptiveDifficulty(recentCreations int) int {
	baseDifficulty := p.difficulty

	// Increase difficulty progressively for high activity
	// This makes spam attacks exponentially more expensive
	if recentCreations > 100 {
		// More than 100 creations in an hour = likely spam
		// Increase by 2 bits (4x harder)
		return min(baseDifficulty+2, 20) // Cap at maximum 20
	}
	if recentCreations > 50 {
		// 50-100 creations = suspicious activity
		// Increase by 1 bit (2x harder)
		return min(baseDifficulty+1, 20)
	}
	if recentCreations > 20 {
		// 20-50 creations = elevated activity
		// Increase by 1 bit if base is already high
		if baseDifficulty >= 16 {
			return min(baseDifficulty+1, 20)
		}
	}

	// Normal activity - use base difficulty
	return baseDifficulty
}

// min returns the minimum of two integers
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
