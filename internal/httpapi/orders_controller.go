package httpapi

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/srjung/debezium-test/internal/domain"
	"github.com/srjung/debezium-test/internal/service"
)

type OrderController struct {
	service service.OrderService
}

func NewOrderController(service service.OrderService) *OrderController {
	return &OrderController{service: service}
}

type createOrderRequest struct {
	CustomerID      string                 `json:"customerId"`
	Status          string                 `json:"status"`
	Currency        string                 `json:"currency"`
	Items           []domain.OrderItem     `json:"items"`
	ShippingAddress domain.ShippingAddress `json:"shippingAddress"`
}

type updateOrderRequest struct {
	CustomerID      string                 `json:"customerId"`
	Status          string                 `json:"status"`
	Currency        string                 `json:"currency"`
	Items           []domain.OrderItem     `json:"items"`
	ShippingAddress domain.ShippingAddress `json:"shippingAddress"`
	ExpectedVersion int64                  `json:"expectedVersion"`
}

func (controller *OrderController) Create(c *gin.Context) {
	key, ok := idempotencyKey(c)
	if !ok {
		return
	}

	var request createOrderRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		writeError(c, err)
		return
	}

	order, err := controller.service.Create(c.Request.Context(), service.CreateOrderCommand{
		CustomerID:      request.CustomerID,
		Status:          request.Status,
		Currency:        request.Currency,
		Items:           request.Items,
		ShippingAddress: request.ShippingAddress,
		IdempotencyKey:  key,
	})
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusCreated, order)
}

func (controller *OrderController) Get(c *gin.Context) {
	order, err := controller.service.Get(c.Request.Context(), c.Param("id"))
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, order)
}

func (controller *OrderController) List(c *gin.Context) {
	limit := 20
	if rawLimit := c.Query("limit"); rawLimit != "" {
		parsedLimit, err := strconv.Atoi(rawLimit)
		if err != nil {
			writeError(c, service.ErrInvalid)
			return
		}
		limit = parsedLimit
	}

	result, err := controller.service.List(c.Request.Context(), service.ListOrdersQuery{
		CustomerID: c.Query("customerId"),
		Status:     c.Query("status"),
		PageToken:  c.Query("pageToken"),
		Limit:      limit,
	})
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, result)
}

func (controller *OrderController) Update(c *gin.Context) {
	key, ok := idempotencyKey(c)
	if !ok {
		return
	}

	var request updateOrderRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		writeError(c, err)
		return
	}

	order, err := controller.service.Update(c.Request.Context(), c.Param("id"), service.UpdateOrderCommand{
		CustomerID:      request.CustomerID,
		Status:          request.Status,
		Currency:        request.Currency,
		Items:           request.Items,
		ShippingAddress: request.ShippingAddress,
		ExpectedVersion: request.ExpectedVersion,
		IdempotencyKey:  key,
	})
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, order)
}

func (controller *OrderController) Delete(c *gin.Context) {
	key, ok := idempotencyKey(c)
	if !ok {
		return
	}

	expectedVersion, err := strconv.ParseInt(c.Query("expectedVersion"), 10, 64)
	if err != nil || expectedVersion < 1 {
		writeError(c, service.ErrInvalid)
		return
	}

	if err := controller.service.Delete(c.Request.Context(), c.Param("id"), expectedVersion, key); err != nil {
		writeError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}
