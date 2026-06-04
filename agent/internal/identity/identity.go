package identity

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"
)

const Version = "1.0.0"

type Identity struct {
	ServerID     string    `json:"server_id"`
	Hostname     string    `json:"hostname"`
	OS           string    `json:"os"`
	Arch         string    `json:"arch"`
	AgentVersion string    `json:"agent_version"`
	CreatedAt    time.Time `json:"created_at"`
}

func New(dir string) (*Identity, error) {
	path := filepath.Join(dir, "identity.json")
	if data, err := os.ReadFile(path); err == nil {
		var id Identity
		if err := json.Unmarshal(data, &id); err != nil {
			return nil, fmt.Errorf("identity: corrupt identity file: %w", err)
		}
		return &id, nil
	}

	hostname, err := os.Hostname()
	if err != nil {
		return nil, fmt.Errorf("identity: hostname: %w", err)
	}

	sid, err := genID()
	if err != nil {
		return nil, fmt.Errorf("identity: generate id: %w", err)
	}

	id := &Identity{
		ServerID:     "srv_" + sid,
		Hostname:     hostname,
		OS:           runtime.GOOS,
		Arch:         runtime.GOARCH,
		AgentVersion: Version,
		CreatedAt:    time.Now().UTC(),
	}

	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("identity: mkdir: %w", err)
	}
	data, err := json.MarshalIndent(id, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("identity: marshal: %w", err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		return nil, fmt.Errorf("identity: write: %w", err)
	}
	return id, nil
}

func (id *Identity) Validate() error {
	if id.ServerID == "" {
		return fmt.Errorf("ServerID is empty")
	}
	if len(id.ServerID) < 4 || id.ServerID[:4] != "srv_" {
		return fmt.Errorf("ServerID must start with 'srv_'")
	}
	if id.Hostname == "" {
		return fmt.Errorf("Hostname is empty")
	}
	return nil
}

func genID() (string, error) {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
