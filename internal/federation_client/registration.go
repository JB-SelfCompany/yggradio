package federation_client

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// RegistrationRequest represents the registration payload sent to federation server
type RegistrationRequest struct {
	UUID        string `json:"uuid"`
	Address     string `json:"address"`
	Port        int    `json:"port"`
	Pubkey      string `json:"pubkey"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Version     string `json:"version"`
	Signature   string `json:"signature"`
	Timestamp   int64  `json:"timestamp"`
}

// RegistrationResponse represents the response from federation server
type RegistrationResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

// register registers this node with the federation server
func (c *Client) register() error {
	// Create registration request (logging removed to reduce noise - only errors are logged)
	timestamp := time.Now().Unix()

	// Build signature message: UUID+ADDRESS+PORT+TIMESTAMP (must match server verification)
	signatureMessage := fmt.Sprintf("%s%s%d%d", c.nodeUUID, c.localAddress, c.serverConfig.Port, timestamp)
	signature := c.crypto.SignMessage(c.localPrivkey, signatureMessage)

	req := RegistrationRequest{
		UUID:        c.nodeUUID,
		Address:     c.localAddress,
		Port:        c.serverConfig.Port,
		Pubkey:      hex.EncodeToString(c.localPubkey),
		Name:        c.serverConfig.InstanceName,
		Description: c.serverConfig.InstanceDescription,
		Version:     c.version,
		Signature:   signature,
		Timestamp:   timestamp,
	}

	// Marshal request to JSON
	reqBody, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("failed to marshal registration request: %w", err)
	}

	// Send request to federation server
	url := fmt.Sprintf("%s/api/federation/register", c.GetServerURL())
	httpReq, err := http.NewRequestWithContext(c.ctx, http.MethodPost, url, bytes.NewReader(reqBody))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")

	// Execute request
	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("failed to send registration request: %w", err)
	}
	defer resp.Body.Close()

	// Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response body: %w", err)
	}

	// Check HTTP status
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("registration failed with status %d: %s", resp.StatusCode, string(body))
	}

	// Parse response
	var regResp RegistrationResponse
	if err := json.Unmarshal(body, &regResp); err != nil {
		return fmt.Errorf("failed to parse registration response: %w", err)
	}

	if !regResp.Success {
		return fmt.Errorf("registration failed: %s", regResp.Message)
	}

	// Successfully registered (no logging to reduce noise - only errors logged)
	return nil
}
