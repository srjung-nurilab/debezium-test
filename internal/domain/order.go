package domain

import "time"

type Order struct {
	ID              string          `json:"id"`
	CustomerID      string          `json:"customerId"`
	Status          string          `json:"status"`
	Currency        string          `json:"currency"`
	Items           []OrderItem     `json:"items"`
	ShippingAddress ShippingAddress `json:"shippingAddress"`
	TotalAmount     string          `json:"totalAmount"`
	Version         int64           `json:"version"`
	LastRequestID   string          `json:"lastRequestId"`
	CreatedAt       time.Time       `json:"createdAt"`
	UpdatedAt       time.Time       `json:"updatedAt"`
}

type OrderItem struct {
	SKU       string `json:"sku"`
	Name      string `json:"name"`
	Quantity  int    `json:"quantity"`
	UnitPrice string `json:"unitPrice"`
}

type ShippingAddress struct {
	Recipient  string `json:"recipient"`
	PostalCode string `json:"postalCode"`
	Address1   string `json:"address1"`
	Address2   string `json:"address2"`
}
