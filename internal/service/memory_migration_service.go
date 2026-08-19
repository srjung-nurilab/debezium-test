package service

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/srjung/debezium-test/internal/domain"
)

type MemoryMigrationService struct {
	mu   sync.RWMutex
	runs map[string]domain.MigrationRun
}

func NewMemoryMigrationService() *MemoryMigrationService {
	return &MemoryMigrationService{runs: make(map[string]domain.MigrationRun)}
}

func (s *MemoryMigrationService) Create(_ context.Context) (domain.MigrationRun, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().UTC()
	run := domain.MigrationRun{
		ID:        fmt.Sprintf("migration-%d", now.UnixNano()),
		State:     domain.MigrationStateCDCBuffering,
		CreatedAt: now,
		UpdatedAt: now,
	}
	s.runs[run.ID] = run
	return run, nil
}

func (s *MemoryMigrationService) Get(_ context.Context, id string) (domain.MigrationRun, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	run, ok := s.runs[id]
	if !ok {
		return domain.MigrationRun{}, ErrNotFound
	}
	return run, nil
}

func (s *MemoryMigrationService) StartBulk(_ context.Context, id string) (domain.MigrationRun, error) {
	return s.transition(id, domain.MigrationStateCDCBuffering, domain.MigrationStateBulkLoading)
}

func (s *MemoryMigrationService) StartReplay(_ context.Context, id string) (domain.MigrationRun, error) {
	return s.transition(id, domain.MigrationStateBulkLoading, domain.MigrationStateCDCReplaying)
}

func (s *MemoryMigrationService) StartValidation(_ context.Context, id string) (domain.MigrationRun, error) {
	return s.transition(id, domain.MigrationStateCDCReplaying, domain.MigrationStateShadowValidating)
}

func (s *MemoryMigrationService) StartCutover(_ context.Context, id string) (domain.MigrationRun, error) {
	return s.transition(id, domain.MigrationStateShadowValidating, domain.MigrationStateCutoverQueuing)
}

func (s *MemoryMigrationService) transition(id string, expected, next domain.MigrationState) (domain.MigrationRun, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	run, ok := s.runs[id]
	if !ok {
		return domain.MigrationRun{}, ErrNotFound
	}
	if run.State != expected {
		return domain.MigrationRun{}, fmt.Errorf("%w: expected state %s, current state %s", ErrConflict, expected, run.State)
	}
	run.State = next
	run.UpdatedAt = time.Now().UTC()
	s.runs[id] = run
	return run, nil
}
