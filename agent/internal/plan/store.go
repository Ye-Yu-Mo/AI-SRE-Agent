package plan

import (
	"fmt"
	"sync"

	"github.com/ai-sre/agent/internal/action"
)

// Store 是 plan 的内存存储。M2 阶段用内存，后续 M4 迁 SQLite。
type Store struct {
	mu    sync.RWMutex
	plans map[string]*action.Plan
}

func NewStore() *Store {
	return &Store{plans: make(map[string]*action.Plan)}
}

func (s *Store) Create(p *action.Plan) {
	s.mu.Lock()
	defer s.mu.Unlock()
	// copy 防止外部修改
	cp := *p
	cp.Steps = make([]action.ActionStep, len(p.Steps))
	copy(cp.Steps, p.Steps)
	s.plans[p.ID] = &cp
}

func (s *Store) Get(id string) (*action.Plan, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	p, ok := s.plans[id]
	if !ok {
		return nil, false
	}
	cp := *p
	return &cp, true
}

func (s *Store) UpdateStatus(id string, status action.PlanStatus) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	p, ok := s.plans[id]
	if !ok {
		return fmt.Errorf("plan %s not found", id)
	}

	if !p.CanTransitionTo(status) {
		return fmt.Errorf("invalid transition: %s → %s", p.Status, status)
	}

	p.Status = status
	return nil
}

func (s *Store) List() []*action.Plan {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]*action.Plan, 0, len(s.plans))
	for _, p := range s.plans {
		cp := *p
		result = append(result, &cp)
	}
	return result
}
