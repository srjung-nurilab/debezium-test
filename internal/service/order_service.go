package service

import (
	"context"

	"github.com/srjung/debezium-test/internal/domain"
)

type CreateOrderCommand struct {
	CustomerID      string
	Status          string
	Currency        string
	Items           []domain.OrderItem
	ShippingAddress domain.ShippingAddress
	IdempotencyKey  string
}

type UpdateOrderCommand struct {
	CustomerID      string
	Status          string
	Currency        string
	Items           []domain.OrderItem
	ShippingAddress domain.ShippingAddress
	ExpectedVersion int64
	IdempotencyKey  string
}

type ListOrdersQuery struct {
	CustomerID string
	Status     string
	PageToken  string
	Limit      int
}

type ListOrdersResult struct {
	Items         []domain.Order `json:"items"`
	NextPageToken string         `json:"nextPageToken,omitempty"`
}

type OrderService interface {
	Create(context.Context, CreateOrderCommand) (domain.Order, error)
	Get(context.Context, string) (domain.Order, error)
	List(context.Context, ListOrdersQuery) (ListOrdersResult, error)
	Update(context.Context, string, UpdateOrderCommand) (domain.Order, error)
	Delete(context.Context, string, int64, string) error
}
