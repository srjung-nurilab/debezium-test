package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/srjung/debezium-test/internal/domain"
)

type MemoryOrderService struct {
	mu              sync.RWMutex
	orders          map[string]domain.Order
	requestOutcomes map[string]string
}

func NewMemoryOrderService() *MemoryOrderService {
	return &MemoryOrderService{
		orders:          make(map[string]domain.Order),
		requestOutcomes: make(map[string]string),
	}
}

func (s *MemoryOrderService) Create(_ context.Context, command CreateOrderCommand) (domain.Order, error) {
	if err := validateOrder(command.CustomerID, command.Status, command.Currency, command.Items, command.ShippingAddress); err != nil {
		return domain.Order{}, err
	}
	if command.Status != "PENDING" {
		return domain.Order{}, fmt.Errorf("%w: new orders must start as PENDING", ErrInvalid)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	requestKey := "POST:/orders:" + command.IdempotencyKey
	if id, ok := s.requestOutcomes[requestKey]; ok {
		return cloneOrder(s.orders[id]), nil
	}

	total, err := calculateTotal(command.Items)
	if err != nil {
		return domain.Order{}, err
	}

	now := time.Now().UTC()
	order := domain.Order{
		ID:              newOrderID(),
		CustomerID:      command.CustomerID,
		Status:          command.Status,
		Currency:        strings.ToUpper(command.Currency),
		Items:           cloneItems(command.Items),
		ShippingAddress: command.ShippingAddress,
		TotalAmount:     total,
		Version:         1,
		LastRequestID:   command.IdempotencyKey,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	s.orders[order.ID] = order
	s.requestOutcomes[requestKey] = order.ID
	return cloneOrder(order), nil
}

func (s *MemoryOrderService) Get(_ context.Context, id string) (domain.Order, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	order, ok := s.orders[id]
	if !ok {
		return domain.Order{}, ErrNotFound
	}
	return cloneOrder(order), nil
}

func (s *MemoryOrderService) List(_ context.Context, query ListOrdersQuery) (ListOrdersResult, error) {
	if query.Limit <= 0 || query.Limit > 100 {
		return ListOrdersResult{}, fmt.Errorf("%w: limit must be between 1 and 100", ErrInvalid)
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	ids := make([]string, 0, len(s.orders))
	for id, order := range s.orders {
		if query.CustomerID != "" && order.CustomerID != query.CustomerID {
			continue
		}
		if query.Status != "" && order.Status != query.Status {
			continue
		}
		if query.PageToken != "" && id <= query.PageToken {
			continue
		}
		ids = append(ids, id)
	}
	sort.Strings(ids)

	result := ListOrdersResult{Items: make([]domain.Order, 0, min(query.Limit, len(ids)))}
	for index, id := range ids {
		if index == query.Limit {
			result.NextPageToken = ids[index-1]
			break
		}
		result.Items = append(result.Items, cloneOrder(s.orders[id]))
	}
	return result, nil
}

func (s *MemoryOrderService) Update(_ context.Context, id string, command UpdateOrderCommand) (domain.Order, error) {
	if err := validateOrder(command.CustomerID, command.Status, command.Currency, command.Items, command.ShippingAddress); err != nil {
		return domain.Order{}, err
	}
	if command.ExpectedVersion < 1 {
		return domain.Order{}, fmt.Errorf("%w: expectedVersion must be positive", ErrInvalid)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	requestKey := "PUT:/orders/" + id + ":" + command.IdempotencyKey
	if previousID, ok := s.requestOutcomes[requestKey]; ok && previousID == id {
		order, exists := s.orders[id]
		if !exists {
			return domain.Order{}, ErrNotFound
		}
		return cloneOrder(order), nil
	}

	order, ok := s.orders[id]
	if !ok {
		return domain.Order{}, ErrNotFound
	}
	if order.Version != command.ExpectedVersion {
		return domain.Order{}, fmt.Errorf("%w: expected version %d, current version %d", ErrConflict, command.ExpectedVersion, order.Version)
	}
	if !validTransition(order.Status, command.Status) {
		return domain.Order{}, fmt.Errorf("%w: %s cannot transition to %s", ErrInvalid, order.Status, command.Status)
	}

	total, err := calculateTotal(command.Items)
	if err != nil {
		return domain.Order{}, err
	}
	order.CustomerID = command.CustomerID
	order.Status = command.Status
	order.Currency = strings.ToUpper(command.Currency)
	order.Items = cloneItems(command.Items)
	order.ShippingAddress = command.ShippingAddress
	order.TotalAmount = total
	order.Version++
	order.LastRequestID = command.IdempotencyKey
	order.UpdatedAt = time.Now().UTC()
	s.orders[id] = order
	s.requestOutcomes[requestKey] = id
	return cloneOrder(order), nil
}

func (s *MemoryOrderService) Delete(_ context.Context, id string, expectedVersion int64, idempotencyKey string) error {
	if expectedVersion < 1 {
		return fmt.Errorf("%w: expectedVersion must be positive", ErrInvalid)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	requestKey := "DELETE:/orders/" + id + ":" + idempotencyKey
	if _, ok := s.requestOutcomes[requestKey]; ok {
		return nil
	}

	order, ok := s.orders[id]
	if !ok {
		return ErrNotFound
	}
	if order.Version != expectedVersion {
		return fmt.Errorf("%w: expected version %d, current version %d", ErrConflict, expectedVersion, order.Version)
	}

	delete(s.orders, id)
	s.requestOutcomes[requestKey] = id
	return nil
}

func newOrderID() string {
	bytes := make([]byte, 12)
	if _, err := rand.Read(bytes); err != nil {
		return fmt.Sprintf("order-%d", time.Now().UTC().UnixNano())
	}
	return hex.EncodeToString(bytes)
}
