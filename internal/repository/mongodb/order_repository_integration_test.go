package mongodb

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/srjung/debezium-test/internal/domain"
	"github.com/srjung/debezium-test/internal/service"
)

func TestOrderRepositoryIntegration(t *testing.T) {
	if os.Getenv("RUN_MONGODB_INTEGRATION") != "1" {
		t.Skip("set RUN_MONGODB_INTEGRATION=1 to run against a MongoDB replica set")
	}

	uri := os.Getenv("MONGODB_URI")
	if uri == "" {
		uri = "mongodb://localhost:27017/?directConnection=true"
	}
	database := os.Getenv("MONGODB_TEST_DATABASE")
	if database == "" {
		database = "app_repository_test"
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	repository, client, err := Connect(ctx, uri, database)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Disconnect(context.Background())
	defer client.Database(database).Drop(context.Background())
	if err := repository.EnsureIndexes(ctx); err != nil {
		t.Fatal(err)
	}

	createCommand := service.CreateOrderCommand{
		CustomerID: "customer-001",
		Status:     "PENDING",
		Currency:   "KRW",
		Items: []domain.OrderItem{{
			SKU: "SKU-001", Name: "상품", Quantity: 2, UnitPrice: "15000.0000",
		}},
		ShippingAddress: domain.ShippingAddress{Recipient: "홍길동", PostalCode: "06236", Address1: "서울시"},
		IdempotencyKey:  "create-1",
	}
	created, err := repository.Create(ctx, createCommand)
	if err != nil {
		t.Fatal(err)
	}
	if created.TotalAmount != "30000.0000" || created.Version != 1 {
		t.Fatalf("unexpected created order: %+v", created)
	}

	duplicate, err := repository.Create(ctx, createCommand)
	if err != nil {
		t.Fatal(err)
	}
	if duplicate.ID != created.ID {
		t.Fatalf("duplicate create returned %s, want %s", duplicate.ID, created.ID)
	}

	updated, err := repository.Update(ctx, created.ID, service.UpdateOrderCommand{
		CustomerID: "customer-001",
		Status:     "CONFIRMED",
		Currency:   "KRW",
		Items: []domain.OrderItem{{
			SKU: "SKU-001", Name: "상품", Quantity: 3, UnitPrice: "15000.0000",
		}},
		ShippingAddress: domain.ShippingAddress{Recipient: "홍길동", PostalCode: "06236", Address1: "서울시"},
		ExpectedVersion: 1,
		IdempotencyKey:  "update-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Version != 2 || updated.TotalAmount != "45000.0000" {
		t.Fatalf("unexpected updated order: %+v", updated)
	}

	if err := repository.Delete(ctx, created.ID, 2, "delete-1"); err != nil {
		t.Fatal(err)
	}
	if err := repository.Delete(ctx, created.ID, 2, "delete-1"); err != nil {
		t.Fatalf("duplicate delete: %v", err)
	}
	if _, err := repository.Get(ctx, created.ID); !errors.Is(err, service.ErrNotFound) {
		t.Fatalf("get after delete error = %v, want not found", err)
	}
}
