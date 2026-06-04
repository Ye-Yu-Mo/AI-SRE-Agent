package storage

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Store 是基于 JSON 文件的本地存储。Phase 0 用文件替代 SQLite，
// 零外部依赖，后续 M4 迁 SQLite/Postgres。
type Store struct {
	mu   sync.RWMutex
	dir  string
}

func NewStore(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("storage: mkdir: %w", err)
	}
	return &Store{dir: dir}, nil
}

// ── State Snapshots ──

type StateSnapshot struct {
	ServerID    string          `json:"server_id"`
	State       json.RawMessage `json:"state"`
	CollectedAt string          `json:"collected_at"`
}

func (s *Store) SaveSnapshot(snap StateSnapshot) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return appendJSON(filepath.Join(s.dir, "snapshots.jsonl"), snap)
}

// ── Audit Events ──

type AuditEvent struct {
	ServerID    string `json:"server_id"`
	PlanID      string `json:"plan_id,omitempty"`
	ActionID    string `json:"action_id"`
	ActionType  string `json:"action_type"`
	Target      string `json:"target"`
	Risk        string `json:"risk,omitempty"`
	Result      string `json:"result"`
	BeforeState string `json:"before_state,omitempty"`
	AfterState  string `json:"after_state,omitempty"`
	Stdout      string `json:"stdout,omitempty"`
	Stderr      string `json:"stderr,omitempty"`
	CreatedAt   string `json:"created_at"`
}

func (s *Store) RecordAudit(e AuditEvent) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if e.CreatedAt == "" {
		e.CreatedAt = time.Now().UTC().Format(time.RFC3339)
	}
	return appendJSON(filepath.Join(s.dir, "audit.jsonl"), e)
}

func (s *Store) SearchAudit(serverID, actionType, result string, limit int) ([]AuditEvent, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	data, err := os.ReadFile(filepath.Join(s.dir, "audit.jsonl"))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var events []AuditEvent
	lines := splitLines(string(data))
	for i := len(lines) - 1; i >= 0; i-- {
		var e AuditEvent
		if err := json.Unmarshal([]byte(lines[i]), &e); err != nil {
			continue
		}
		if serverID != "" && e.ServerID != serverID {
			continue
		}
		if actionType != "" && e.ActionType != actionType {
			continue
		}
		if result != "" && e.Result != result {
			continue
		}
		events = append(events, e)
		if limit > 0 && len(events) >= limit {
			break
		}
	}
	return events, nil
}

// ── helpers ──

func appendJSON(path string, v interface{}) error {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	b, _ := json.Marshal(v)
	_, err = f.Write(append(b, '\n'))
	return err
}

func splitLines(s string) []string {
	var lines []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			if i > start {
				lines = append(lines, s[start:i])
			}
			start = i + 1
		}
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}
