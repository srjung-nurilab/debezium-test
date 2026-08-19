package service

import (
	"fmt"
	"math/big"
	"strings"

	"github.com/srjung/debezium-test/internal/domain"
)

func validateOrder(customerID, status, currency string, items []domain.OrderItem, address domain.ShippingAddress) error {
	if strings.TrimSpace(customerID) == "" {
		return fmt.Errorf("%w: customerId is required", ErrInvalid)
	}
	if !validStatus(status) {
		return fmt.Errorf("%w: unsupported status", ErrInvalid)
	}
	if len(currency) != 3 || strings.ToUpper(currency) != currency {
		return fmt.Errorf("%w: currency must be a three-letter uppercase code", ErrInvalid)
	}
	if len(items) == 0 {
		return fmt.Errorf("%w: at least one item is required", ErrInvalid)
	}
	if strings.TrimSpace(address.Recipient) == "" || strings.TrimSpace(address.PostalCode) == "" || strings.TrimSpace(address.Address1) == "" {
		return fmt.Errorf("%w: shippingAddress recipient, postalCode, and address1 are required", ErrInvalid)
	}
	for _, item := range items {
		if strings.TrimSpace(item.SKU) == "" || strings.TrimSpace(item.Name) == "" || item.Quantity <= 0 || strings.TrimSpace(item.UnitPrice) == "" {
			return fmt.Errorf("%w: each item requires sku, name, positive quantity, and unitPrice", ErrInvalid)
		}
	}
	return nil
}

func calculateTotal(items []domain.OrderItem) (string, error) {
	total := new(big.Rat)
	for _, item := range items {
		price, ok := new(big.Rat).SetString(item.UnitPrice)
		if !ok || price.Sign() < 0 {
			return "", fmt.Errorf("%w: unitPrice must be a non-negative decimal", ErrInvalid)
		}
		lineTotal := new(big.Rat).Mul(price, big.NewRat(int64(item.Quantity), 1))
		total.Add(total, lineTotal)
	}
	return total.FloatString(4), nil
}

func validStatus(status string) bool {
	switch status {
	case "PENDING", "CONFIRMED", "CANCELLED", "SHIPPED":
		return true
	default:
		return false
	}
}

func validTransition(from, to string) bool {
	if from == to {
		return true
	}
	switch from {
	case "PENDING":
		return to == "CONFIRMED" || to == "CANCELLED"
	case "CONFIRMED":
		return to == "SHIPPED" || to == "CANCELLED"
	default:
		return false
	}
}

func cloneOrder(order domain.Order) domain.Order {
	order.Items = cloneItems(order.Items)
	return order
}

func cloneItems(items []domain.OrderItem) []domain.OrderItem {
	return append([]domain.OrderItem(nil), items...)
}

func min(left, right int) int {
	if left < right {
		return left
	}
	return right
}
