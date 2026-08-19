package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/srjung/debezium-test/internal/service"
)

func TestOrderCRUDRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := NewRouter(service.NewMemoryOrderService(), service.NewMemoryMigrationService())

	create := performRequest(t, router, http.MethodPost, "/orders", map[string]any{
		"customerId": "customer-001",
		"status":     "PENDING",
		"currency":   "KRW",
		"items": []map[string]any{{
			"sku": "SKU-001", "name": "상품", "quantity": 2, "unitPrice": "15000.0000",
		}},
		"shippingAddress": map[string]any{
			"recipient": "홍길동", "postalCode": "06236", "address1": "서울시", "address2": "101호",
		},
	}, map[string]string{"Idempotency-Key": "request-create-1"})
	if create.Code != http.StatusCreated {
		t.Fatalf("create status = %d, body = %s", create.Code, create.Body.String())
	}

	var order struct {
		ID          string `json:"id"`
		Version     int64  `json:"version"`
		TotalAmount string `json:"totalAmount"`
	}
	decodeBody(t, create, &order)
	if order.TotalAmount != "30000.0000" {
		t.Fatalf("total amount = %s", order.TotalAmount)
	}

	get := performRequest(t, router, http.MethodGet, "/orders/"+order.ID, nil, nil)
	if get.Code != http.StatusOK {
		t.Fatalf("get status = %d, body = %s", get.Code, get.Body.String())
	}

	update := performRequest(t, router, http.MethodPut, "/orders/"+order.ID, map[string]any{
		"customerId":      "customer-001",
		"status":          "CONFIRMED",
		"currency":        "KRW",
		"expectedVersion": order.Version,
		"items": []map[string]any{{
			"sku": "SKU-001", "name": "상품", "quantity": 3, "unitPrice": "15000.0000",
		}},
		"shippingAddress": map[string]any{
			"recipient": "홍길동", "postalCode": "06236", "address1": "서울시",
		},
	}, map[string]string{"Idempotency-Key": "request-update-1"})
	if update.Code != http.StatusOK {
		t.Fatalf("update status = %d, body = %s", update.Code, update.Body.String())
	}

	remove := performRequest(t, router, http.MethodDelete, "/orders/"+order.ID+"?expectedVersion=2", nil, map[string]string{"Idempotency-Key": "request-delete-1"})
	if remove.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d, body = %s", remove.Code, remove.Body.String())
	}
}

func TestOrderWriteRequiresIdempotencyKey(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := NewRouter(service.NewMemoryOrderService(), service.NewMemoryMigrationService())

	response := performRequest(t, router, http.MethodPost, "/orders", map[string]any{}, nil)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
}

func TestMigrationRoutesFollowStateMachine(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := NewRouter(service.NewMemoryOrderService(), service.NewMemoryMigrationService())

	create := performRequest(t, router, http.MethodPost, "/admin/migrations", nil, nil)
	if create.Code != http.StatusCreated {
		t.Fatalf("create status = %d", create.Code)
	}
	var run struct {
		ID    string `json:"id"`
		State string `json:"state"`
	}
	decodeBody(t, create, &run)
	if run.State != "CDC_BUFFERING" {
		t.Fatalf("initial state = %s", run.State)
	}

	for _, path := range []string{"/bulk", "/replay", "/validate", "/cutover"} {
		response := performRequest(t, router, http.MethodPost, "/admin/migrations/"+run.ID+path, nil, nil)
		if response.Code != http.StatusAccepted {
			t.Fatalf("%s status = %d, body = %s", path, response.Code, response.Body.String())
		}
	}
}

func performRequest(t *testing.T, router http.Handler, method, path string, body any, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()

	var payload []byte
	if body != nil {
		var err error
		payload, err = json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
	}
	request := httptest.NewRequest(method, path, bytes.NewReader(payload))
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	for key, value := range headers {
		request.Header.Set(key, value)
	}
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}

func decodeBody(t *testing.T, response *httptest.ResponseRecorder, target any) {
	t.Helper()
	if err := json.Unmarshal(response.Body.Bytes(), target); err != nil {
		t.Fatal(err)
	}
}
