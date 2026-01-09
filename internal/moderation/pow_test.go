package moderation

import (
	"testing"
	"time"
)

func TestNewProofOfWork(t *testing.T) {
	tests := []struct {
		name       string
		difficulty int
		expected   int
	}{
		{"valid difficulty", 10, 10},
		{"too low clamped", 2, 4},
		{"too high clamped", 25, 20},
		{"minimum", 4, 4},
		{"maximum", 20, 20},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pow := NewProofOfWork(tt.difficulty)
			if pow.GetDifficulty() != tt.expected {
				t.Errorf("expected difficulty %d, got %d", tt.expected, pow.GetDifficulty())
			}
		})
	}
}

func TestProofOfWork_GenerateChallenge(t *testing.T) {
	pow := NewProofOfWork(8)
	pubkey := "testpubkey123"

	challenge := pow.GenerateChallenge(pubkey)

	if challenge.Pubkey != pubkey {
		t.Errorf("expected pubkey %s, got %s", pubkey, challenge.Pubkey)
	}

	if challenge.Difficulty != 8 {
		t.Errorf("expected difficulty 8, got %d", challenge.Difficulty)
	}

	if challenge.Timestamp == 0 {
		t.Error("timestamp should not be zero")
	}

	// Timestamp should be recent (within last second)
	now := time.Now().Unix()
	if challenge.Timestamp < now-1 || challenge.Timestamp > now+1 {
		t.Errorf("timestamp not recent: %d (now: %d)", challenge.Timestamp, now)
	}
}

func TestProofOfWork_Verify(t *testing.T) {
	pow := NewProofOfWork(4) // Low difficulty for testing

	challenge := pow.GenerateChallenge("testpubkey")

	// Solve the challenge
	nonce, err := pow.Solve(challenge)
	if err != nil {
		t.Fatalf("failed to solve challenge: %v", err)
	}

	// Verify the solution
	if !pow.Verify(challenge, nonce) {
		t.Error("valid solution not verified")
	}

	// Verify with wrong nonce
	if pow.Verify(challenge, "wrongnonce") {
		t.Error("invalid solution verified")
	}
}

func TestProofOfWork_TimestampValidation(t *testing.T) {
	pow := NewProofOfWork(4)

	tests := []struct {
		name      string
		timestamp int64
		valid     bool
	}{
		{"current time", time.Now().Unix(), true},
		{"5 minutes ago", time.Now().Unix() - 300, true},
		{"6 minutes ago", time.Now().Unix() - 360, false},
		{"1 minute future", time.Now().Unix() + 60, true},
		{"2 minutes future", time.Now().Unix() + 120, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			challenge := &Challenge{
				Pubkey:     "testpubkey",
				Timestamp:  tt.timestamp,
				Difficulty: 4,
			}

			// Solve PoW for the specific timestamp
			nonce, err := pow.Solve(challenge)

			// For old timestamps, solving may fail due to timestamp validation
			// In that case, we just test timestamp validation logic directly
			if err != nil && !tt.valid {
				// Expected to fail for invalid timestamps
				return
			}
			if err != nil && tt.valid {
				t.Fatalf("failed to solve for valid timestamp: %v", err)
			}

			// Verify with the same challenge
			result := pow.Verify(challenge, nonce)
			if result != tt.valid {
				t.Errorf("expected valid=%v, got %v", tt.valid, result)
			}
		})
	}
}

func TestProofOfWork_Solve(t *testing.T) {
	pow := NewProofOfWork(4) // Low difficulty for testing

	challenge := pow.GenerateChallenge("testpubkey")

	// Should be able to solve low difficulty
	nonce, err := pow.Solve(challenge)
	if err != nil {
		t.Fatalf("failed to solve: %v", err)
	}

	if nonce == "" {
		t.Error("nonce should not be empty")
	}

	// Verify the solution
	if !pow.Verify(challenge, nonce) {
		t.Error("solved nonce not verified")
	}
}

func TestProofOfWork_ComputeHash(t *testing.T) {
	pow := NewProofOfWork(8)

	challenge := &Challenge{
		Pubkey:     "testpubkey",
		Timestamp:  1234567890,
		Difficulty: 8,
	}

	hash1 := pow.ComputeHash(challenge, "123")
	hash2 := pow.ComputeHash(challenge, "123")
	hash3 := pow.ComputeHash(challenge, "456")

	// Same inputs should produce same hash
	if hash1 != hash2 {
		t.Error("same inputs produced different hashes")
	}

	// Different inputs should produce different hashes
	if hash1 == hash3 {
		t.Error("different inputs produced same hash")
	}

	// Hash should be 64 characters (256 bits in hex)
	if len(hash1) != 64 {
		t.Errorf("expected hash length 64, got %d", len(hash1))
	}
}

func TestProofOfWork_ValidateChallenge(t *testing.T) {
	pow := NewProofOfWork(8)

	tests := []struct {
		name      string
		challenge *Challenge
		wantErr   bool
	}{
		{
			name: "valid challenge",
			challenge: &Challenge{
				Pubkey:     "validpubkey",
				Timestamp:  time.Now().Unix(),
				Difficulty: 8,
			},
			wantErr: false,
		},
		{
			name: "missing pubkey",
			challenge: &Challenge{
				Timestamp:  time.Now().Unix(),
				Difficulty: 8,
			},
			wantErr: true,
		},
		{
			name: "missing timestamp",
			challenge: &Challenge{
				Pubkey:     "validpubkey",
				Difficulty: 8,
			},
			wantErr: true,
		},
		{
			name: "difficulty too low",
			challenge: &Challenge{
				Pubkey:     "validpubkey",
				Timestamp:  time.Now().Unix(),
				Difficulty: 2,
			},
			wantErr: true,
		},
		{
			name: "difficulty too high",
			challenge: &Challenge{
				Pubkey:     "validpubkey",
				Timestamp:  time.Now().Unix(),
				Difficulty: 25,
			},
			wantErr: true,
		},
		{
			name: "timestamp too old",
			challenge: &Challenge{
				Pubkey:     "validpubkey",
				Timestamp:  time.Now().Unix() - 400,
				Difficulty: 8,
			},
			wantErr: true,
		},
		{
			name: "timestamp too future",
			challenge: &Challenge{
				Pubkey:     "validpubkey",
				Timestamp:  time.Now().Unix() + 100,
				Difficulty: 8,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := pow.ValidateChallenge(tt.challenge)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateChallenge() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestProofOfWork_VerifyHash(t *testing.T) {
	pow := NewProofOfWork(8)

	tests := []struct {
		name  string
		hash  string
		valid bool
	}{
		{
			name:  "hash with 8 leading zero bits",
			hash:  "00ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff",
			valid: true,
		},
		{
			name:  "hash with no leading zeros",
			hash:  "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff",
			valid: false,
		},
		{
			name:  "invalid hex",
			hash:  "not a valid hash",
			valid: false,
		},
		{
			name:  "wrong length",
			hash:  "00ff",
			valid: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := pow.VerifyHash(tt.hash)
			if result != tt.valid {
				t.Errorf("expected %v, got %v", tt.valid, result)
			}
		})
	}
}

func TestCountLeadingZeroBits(t *testing.T) {
	tests := []struct {
		name     string
		hash     []byte
		expected int
	}{
		{
			name:     "no zeros",
			hash:     []byte{0xff, 0xff, 0xff},
			expected: 0,
		},
		{
			name:     "one zero byte",
			hash:     []byte{0x00, 0xff, 0xff},
			expected: 8,
		},
		{
			name:     "two zero bytes",
			hash:     []byte{0x00, 0x00, 0xff},
			expected: 16,
		},
		{
			name:     "partial zero byte",
			hash:     []byte{0x0f, 0xff, 0xff},
			expected: 4,
		},
		{
			name:     "all zeros",
			hash:     []byte{0x00, 0x00, 0x00, 0x00},
			expected: 32,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := countLeadingZeroBits(tt.hash)
			if result != tt.expected {
				t.Errorf("expected %d, got %d", tt.expected, result)
			}
		})
	}
}

func TestProofOfWork_EstimateTime(t *testing.T) {
	pow := NewProofOfWork(8)

	// Test that estimate increases with difficulty
	time4 := pow.EstimateTime(4)
	time8 := pow.EstimateTime(8)
	time12 := pow.EstimateTime(12)

	if time8 <= time4 {
		t.Error("higher difficulty should take more time")
	}

	if time12 <= time8 {
		t.Error("higher difficulty should take more time")
	}

	// Estimate should be reasonable (not negative or absurdly large)
	if time4 < 0 {
		t.Error("estimate should not be negative")
	}

	if time4 > 1*time.Hour {
		t.Error("estimate for difficulty 4 should be under 1 hour")
	}
}

// Security Tests
func TestProofOfWork_ReplayAttackPrevention(t *testing.T) {
	pow := NewProofOfWork(4)

	// Generate and solve challenge
	challenge := pow.GenerateChallenge("testpubkey")
	nonce, err := pow.Solve(challenge)
	if err != nil {
		t.Fatalf("failed to solve: %v", err)
	}

	// First verification should pass
	if !pow.Verify(challenge, nonce) {
		t.Error("first verification failed")
	}

	// Simulate replay attack with old timestamp
	oldChallenge := &Challenge{
		Pubkey:     challenge.Pubkey,
		Timestamp:  time.Now().Unix() - 400, // 6 minutes ago
		Difficulty: challenge.Difficulty,
	}

	// Replay with old timestamp should fail
	if pow.Verify(oldChallenge, nonce) {
		t.Error("replay attack with old timestamp should fail")
	}
}

func TestProofOfWork_DifficultyEnforcement(t *testing.T) {
	// Create PoW with difficulty 8
	pow := NewProofOfWork(8)

	// Generate challenge
	challenge := pow.GenerateChallenge("testpubkey")

	// Solve with difficulty 8
	nonce, err := pow.Solve(challenge)
	if err != nil {
		t.Fatalf("failed to solve: %v", err)
	}

	// Verify with correct difficulty
	if !pow.Verify(challenge, nonce) {
		t.Error("verification with correct difficulty failed")
	}

	// Try to verify with lower difficulty (should still pass as solution exceeds requirement)
	challengeLowDiff := &Challenge{
		Pubkey:     challenge.Pubkey,
		Timestamp:  challenge.Timestamp,
		Difficulty: 4,
	}
	powLow := NewProofOfWork(4)
	if !powLow.Verify(challengeLowDiff, nonce) {
		t.Error("solution should pass lower difficulty")
	}

	// Try to verify with higher difficulty (may fail)
	challengeHighDiff := &Challenge{
		Pubkey:     challenge.Pubkey,
		Timestamp:  challenge.Timestamp,
		Difficulty: 12,
	}
	powHigh := NewProofOfWork(12)
	// This verification may pass or fail depending on the actual hash
	// We're just testing that the function doesn't panic
	_ = powHigh.Verify(challengeHighDiff, nonce)
}

func TestProofOfWork_ConcurrentSolve(t *testing.T) {
	pow := NewProofOfWork(4)

	// Test that multiple concurrent solves work correctly
	challenges := make([]*Challenge, 5)
	for i := range challenges {
		challenges[i] = pow.GenerateChallenge("testpubkey" + string(rune(i)))
	}

	results := make(chan bool, 5)

	for _, challenge := range challenges {
		go func(ch *Challenge) {
			nonce, err := pow.Solve(ch)
			if err != nil {
				results <- false
				return
			}
			results <- pow.Verify(ch, nonce)
		}(challenge)
	}

	// Collect results
	for i := 0; i < 5; i++ {
		result := <-results
		if !result {
			t.Error("concurrent solve failed")
		}
	}
}
