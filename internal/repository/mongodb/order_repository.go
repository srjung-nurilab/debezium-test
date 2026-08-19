package mongodb

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/srjung/debezium-test/internal/domain"
	"github.com/srjung/debezium-test/internal/service"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readpref"
)

const (
	ordersCollection   = "orders"
	commandsCollection = "order_commands"
)

type OrderRepository struct {
	client   *mongo.Client
	orders   *mongo.Collection
	commands *mongo.Collection
}

type orderDocument struct {
	ID              primitive.ObjectID   `bson:"_id"`
	CustomerID      string               `bson:"customerId"`
	Status          string               `bson:"status"`
	Currency        string               `bson:"currency"`
	Items           []orderItemDocument  `bson:"items"`
	ShippingAddress addressDocument      `bson:"shippingAddress"`
	TotalAmount     primitive.Decimal128 `bson:"totalAmount"`
	Version         int64                `bson:"version"`
	LastRequestID   string               `bson:"lastRequestId"`
	CreatedAt       time.Time            `bson:"createdAt"`
	UpdatedAt       time.Time            `bson:"updatedAt"`
}

type orderItemDocument struct {
	SKU       string               `bson:"sku"`
	Name      string               `bson:"name"`
	Quantity  int                  `bson:"quantity"`
	UnitPrice primitive.Decimal128 `bson:"unitPrice"`
}

type addressDocument struct {
	Recipient  string `bson:"recipient"`
	PostalCode string `bson:"postalCode"`
	Address1   string `bson:"address1"`
	Address2   string `bson:"address2,omitempty"`
}

type commandDocument struct {
	ID        string             `bson:"_id"`
	OrderID   primitive.ObjectID `bson:"orderId"`
	Deleted   bool               `bson:"deleted"`
	Result    *orderDocument     `bson:"result,omitempty"`
	CreatedAt time.Time          `bson:"createdAt"`
}

func Connect(ctx context.Context, uri, database string) (*OrderRepository, *mongo.Client, error) {
	client, err := mongo.Connect(ctx, options.Client().ApplyURI(uri))
	if err != nil {
		return nil, nil, fmt.Errorf("connect MongoDB: %w", err)
	}
	if err := client.Ping(ctx, readpref.Primary()); err != nil {
		_ = client.Disconnect(context.Background())
		return nil, nil, fmt.Errorf("ping MongoDB: %w", err)
	}

	repository := NewOrderRepository(client, database)
	if err := repository.EnsureIndexes(ctx); err != nil {
		_ = client.Disconnect(context.Background())
		return nil, nil, err
	}
	return repository, client, nil
}

func NewOrderRepository(client *mongo.Client, database string) *OrderRepository {
	db := client.Database(database)
	return &OrderRepository{
		client:   client,
		orders:   db.Collection(ordersCollection),
		commands: db.Collection(commandsCollection),
	}
}

func (repository *OrderRepository) EnsureIndexes(ctx context.Context) error {
	_, err := repository.orders.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys: bson.D{
			{Key: "customerId", Value: 1},
			{Key: "status", Value: 1},
			{Key: "_id", Value: 1},
		},
	})
	if err != nil {
		return fmt.Errorf("create orders index: %w", err)
	}
	_, err = repository.commands.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys: bson.D{{Key: "createdAt", Value: 1}},
	})
	if err != nil {
		return fmt.Errorf("create order commands index: %w", err)
	}
	return nil
}

func (repository *OrderRepository) Create(ctx context.Context, command service.CreateOrderCommand) (domain.Order, error) {
	if err := validateCreate(command); err != nil {
		return domain.Order{}, err
	}

	total, err := calculateTotal(command.Items)
	if err != nil {
		return domain.Order{}, err
	}
	now := time.Now().UTC()
	document, err := newOrderDocument(primitive.NewObjectID(), command.CustomerID, command.Status, command.Currency, command.Items, command.ShippingAddress, total, 1, command.IdempotencyKey, now, now)
	if err != nil {
		return domain.Order{}, err
	}

	requestKey := commandKey("POST", "orders", command.IdempotencyKey)
	var result domain.Order
	err = repository.withTransaction(ctx, func(sessionContext mongo.SessionContext) error {
		existing, found, err := repository.findCommand(sessionContext, requestKey)
		if err != nil {
			return err
		}
		if found {
			if existing.Result == nil {
				return fmt.Errorf("%w: idempotency key belongs to a delete request", service.ErrConflict)
			}
			result, err = documentToOrder(*existing.Result)
			return err
		}

		if _, err := repository.orders.InsertOne(sessionContext, document); err != nil {
			return fmt.Errorf("insert order: %w", err)
		}
		if err := repository.insertCommand(sessionContext, commandDocument{ID: requestKey, OrderID: document.ID, Result: &document, CreatedAt: now}); err != nil {
			return err
		}
		result, err = documentToOrder(document)
		return err
	})
	return result, err
}

func (repository *OrderRepository) Get(ctx context.Context, id string) (domain.Order, error) {
	objectID, err := parseOrderID(id)
	if err != nil {
		return domain.Order{}, err
	}

	var document orderDocument
	err = repository.orders.FindOne(ctx, bson.M{"_id": objectID}).Decode(&document)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return domain.Order{}, service.ErrNotFound
	}
	if err != nil {
		return domain.Order{}, fmt.Errorf("find order: %w", err)
	}
	return documentToOrder(document)
}

func (repository *OrderRepository) List(ctx context.Context, query service.ListOrdersQuery) (service.ListOrdersResult, error) {
	if query.Limit <= 0 || query.Limit > 100 {
		return service.ListOrdersResult{}, fmt.Errorf("%w: limit must be between 1 and 100", service.ErrInvalid)
	}

	filter := bson.M{}
	if query.CustomerID != "" {
		filter["customerId"] = query.CustomerID
	}
	if query.Status != "" {
		filter["status"] = query.Status
	}
	if query.PageToken != "" {
		pageToken, err := parseOrderID(query.PageToken)
		if err != nil {
			return service.ListOrdersResult{}, err
		}
		filter["_id"] = bson.M{"$gt": pageToken}
	}

	cursor, err := repository.orders.Find(ctx, filter, options.Find().SetSort(bson.D{{Key: "_id", Value: 1}}).SetLimit(int64(query.Limit+1)))
	if err != nil {
		return service.ListOrdersResult{}, fmt.Errorf("list orders: %w", err)
	}
	defer cursor.Close(ctx)

	result := service.ListOrdersResult{Items: make([]domain.Order, 0, query.Limit)}
	for cursor.Next(ctx) {
		var document orderDocument
		if err := cursor.Decode(&document); err != nil {
			return service.ListOrdersResult{}, fmt.Errorf("decode order: %w", err)
		}
		if len(result.Items) == query.Limit {
			result.NextPageToken = result.Items[len(result.Items)-1].ID
			break
		}
		order, err := documentToOrder(document)
		if err != nil {
			return service.ListOrdersResult{}, err
		}
		result.Items = append(result.Items, order)
	}
	if err := cursor.Err(); err != nil {
		return service.ListOrdersResult{}, fmt.Errorf("iterate orders: %w", err)
	}
	return result, nil
}

func (repository *OrderRepository) Update(ctx context.Context, id string, command service.UpdateOrderCommand) (domain.Order, error) {
	if err := validateUpdate(command); err != nil {
		return domain.Order{}, err
	}
	objectID, err := parseOrderID(id)
	if err != nil {
		return domain.Order{}, err
	}
	total, err := calculateTotal(command.Items)
	if err != nil {
		return domain.Order{}, err
	}

	requestKey := commandKey("PUT", id, command.IdempotencyKey)
	var result domain.Order
	err = repository.withTransaction(ctx, func(sessionContext mongo.SessionContext) error {
		existing, found, err := repository.findCommand(sessionContext, requestKey)
		if err != nil {
			return err
		}
		if found {
			if existing.Result == nil {
				return fmt.Errorf("%w: idempotency key belongs to a delete request", service.ErrConflict)
			}
			result, err = documentToOrder(*existing.Result)
			return err
		}

		var current orderDocument
		err = repository.orders.FindOne(sessionContext, bson.M{"_id": objectID}).Decode(&current)
		if errors.Is(err, mongo.ErrNoDocuments) {
			return service.ErrNotFound
		}
		if err != nil {
			return fmt.Errorf("find order for update: %w", err)
		}
		if current.Version != command.ExpectedVersion {
			return fmt.Errorf("%w: expected version %d, current version %d", service.ErrConflict, command.ExpectedVersion, current.Version)
		}
		if !validTransition(current.Status, command.Status) {
			return fmt.Errorf("%w: %s cannot transition to %s", service.ErrInvalid, current.Status, command.Status)
		}

		next, err := newOrderDocument(current.ID, command.CustomerID, command.Status, command.Currency, command.Items, command.ShippingAddress, total, current.Version+1, command.IdempotencyKey, current.CreatedAt, time.Now().UTC())
		if err != nil {
			return err
		}
		updateResult, err := repository.orders.ReplaceOne(sessionContext, bson.M{"_id": objectID, "version": command.ExpectedVersion}, next)
		if err != nil {
			return fmt.Errorf("replace order: %w", err)
		}
		if updateResult.MatchedCount != 1 {
			return fmt.Errorf("%w: order was updated concurrently", service.ErrConflict)
		}
		if err := repository.insertCommand(sessionContext, commandDocument{ID: requestKey, OrderID: objectID, Result: &next, CreatedAt: next.UpdatedAt}); err != nil {
			return err
		}
		result, err = documentToOrder(next)
		return err
	})
	return result, err
}

func (repository *OrderRepository) Delete(ctx context.Context, id string, expectedVersion int64, idempotencyKey string) error {
	if expectedVersion < 1 {
		return fmt.Errorf("%w: expectedVersion must be positive", service.ErrInvalid)
	}
	objectID, err := parseOrderID(id)
	if err != nil {
		return err
	}

	requestKey := commandKey("DELETE", id, idempotencyKey)
	return repository.withTransaction(ctx, func(sessionContext mongo.SessionContext) error {
		_, found, err := repository.findCommand(sessionContext, requestKey)
		if err != nil {
			return err
		}
		if found {
			return nil
		}

		var current orderDocument
		err = repository.orders.FindOne(sessionContext, bson.M{"_id": objectID}).Decode(&current)
		if errors.Is(err, mongo.ErrNoDocuments) {
			return service.ErrNotFound
		}
		if err != nil {
			return fmt.Errorf("find order for delete: %w", err)
		}
		if current.Version != expectedVersion {
			return fmt.Errorf("%w: expected version %d, current version %d", service.ErrConflict, expectedVersion, current.Version)
		}

		deleteResult, err := repository.orders.DeleteOne(sessionContext, bson.M{"_id": objectID, "version": expectedVersion})
		if err != nil {
			return fmt.Errorf("delete order: %w", err)
		}
		if deleteResult.DeletedCount != 1 {
			return fmt.Errorf("%w: order was updated concurrently", service.ErrConflict)
		}
		return repository.insertCommand(sessionContext, commandDocument{ID: requestKey, OrderID: objectID, Deleted: true, CreatedAt: time.Now().UTC()})
	})
}

func (repository *OrderRepository) withTransaction(ctx context.Context, callback func(mongo.SessionContext) error) error {
	session, err := repository.client.StartSession()
	if err != nil {
		return fmt.Errorf("start MongoDB session: %w", err)
	}
	defer session.EndSession(ctx)

	_, err = session.WithTransaction(ctx, func(sessionContext mongo.SessionContext) (interface{}, error) {
		return nil, callback(sessionContext)
	})
	if err != nil {
		return fmt.Errorf("MongoDB transaction: %w", err)
	}
	return nil
}

func (repository *OrderRepository) findCommand(ctx context.Context, key string) (commandDocument, bool, error) {
	var command commandDocument
	err := repository.commands.FindOne(ctx, bson.M{"_id": key}).Decode(&command)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return commandDocument{}, false, nil
	}
	if err != nil {
		return commandDocument{}, false, fmt.Errorf("find idempotency command: %w", err)
	}
	return command, true, nil
}

func (repository *OrderRepository) insertCommand(ctx context.Context, command commandDocument) error {
	if _, err := repository.commands.InsertOne(ctx, command); err != nil {
		return fmt.Errorf("insert idempotency command: %w", err)
	}
	return nil
}

func validateCreate(command service.CreateOrderCommand) error {
	if err := validateOrder(command.CustomerID, command.Status, command.Currency, command.Items, command.ShippingAddress); err != nil {
		return err
	}
	if command.Status != "PENDING" {
		return fmt.Errorf("%w: new orders must start as PENDING", service.ErrInvalid)
	}
	if strings.TrimSpace(command.IdempotencyKey) == "" {
		return fmt.Errorf("%w: idempotency key is required", service.ErrInvalid)
	}
	return nil
}

func validateUpdate(command service.UpdateOrderCommand) error {
	if err := validateOrder(command.CustomerID, command.Status, command.Currency, command.Items, command.ShippingAddress); err != nil {
		return err
	}
	if command.ExpectedVersion < 1 {
		return fmt.Errorf("%w: expectedVersion must be positive", service.ErrInvalid)
	}
	if strings.TrimSpace(command.IdempotencyKey) == "" {
		return fmt.Errorf("%w: idempotency key is required", service.ErrInvalid)
	}
	return nil
}

func newOrderDocument(id primitive.ObjectID, customerID, status, currency string, items []domain.OrderItem, address domain.ShippingAddress, total string, version int64, requestID string, createdAt, updatedAt time.Time) (orderDocument, error) {
	documentItems := make([]orderItemDocument, 0, len(items))
	for _, item := range items {
		unitPrice, err := primitive.ParseDecimal128(item.UnitPrice)
		if err != nil {
			return orderDocument{}, fmt.Errorf("%w: invalid item unitPrice: %v", service.ErrInvalid, err)
		}
		documentItems = append(documentItems, orderItemDocument{SKU: item.SKU, Name: item.Name, Quantity: item.Quantity, UnitPrice: unitPrice})
	}
	totalAmount, err := primitive.ParseDecimal128(total)
	if err != nil {
		return orderDocument{}, fmt.Errorf("%w: invalid total amount: %v", service.ErrInvalid, err)
	}
	return orderDocument{
		ID:         id,
		CustomerID: customerID,
		Status:     status,
		Currency:   strings.ToUpper(currency),
		Items:      documentItems,
		ShippingAddress: addressDocument{
			Recipient: address.Recipient, PostalCode: address.PostalCode, Address1: address.Address1, Address2: address.Address2,
		},
		TotalAmount:   totalAmount,
		Version:       version,
		LastRequestID: requestID,
		CreatedAt:     createdAt,
		UpdatedAt:     updatedAt,
	}, nil
}

func documentToOrder(document orderDocument) (domain.Order, error) {
	items := make([]domain.OrderItem, 0, len(document.Items))
	for _, item := range document.Items {
		items = append(items, domain.OrderItem{SKU: item.SKU, Name: item.Name, Quantity: item.Quantity, UnitPrice: item.UnitPrice.String()})
	}
	return domain.Order{
		ID:              document.ID.Hex(),
		CustomerID:      document.CustomerID,
		Status:          document.Status,
		Currency:        document.Currency,
		Items:           items,
		ShippingAddress: domain.ShippingAddress{Recipient: document.ShippingAddress.Recipient, PostalCode: document.ShippingAddress.PostalCode, Address1: document.ShippingAddress.Address1, Address2: document.ShippingAddress.Address2},
		TotalAmount:     document.TotalAmount.String(),
		Version:         document.Version,
		LastRequestID:   document.LastRequestID,
		CreatedAt:       document.CreatedAt,
		UpdatedAt:       document.UpdatedAt,
	}, nil
}

func parseOrderID(id string) (primitive.ObjectID, error) {
	objectID, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return primitive.NilObjectID, fmt.Errorf("%w: order id must be a MongoDB ObjectId", service.ErrInvalid)
	}
	return objectID, nil
}

func commandKey(method, id, idempotencyKey string) string {
	return method + ":" + id + ":" + idempotencyKey
}

func validateOrder(customerID, status, currency string, items []domain.OrderItem, address domain.ShippingAddress) error {
	if strings.TrimSpace(customerID) == "" {
		return fmt.Errorf("%w: customerId is required", service.ErrInvalid)
	}
	if !validStatus(status) {
		return fmt.Errorf("%w: unsupported status", service.ErrInvalid)
	}
	if len(currency) != 3 || strings.ToUpper(currency) != currency {
		return fmt.Errorf("%w: currency must be a three-letter uppercase code", service.ErrInvalid)
	}
	if len(items) == 0 {
		return fmt.Errorf("%w: at least one item is required", service.ErrInvalid)
	}
	if strings.TrimSpace(address.Recipient) == "" || strings.TrimSpace(address.PostalCode) == "" || strings.TrimSpace(address.Address1) == "" {
		return fmt.Errorf("%w: shippingAddress recipient, postalCode, and address1 are required", service.ErrInvalid)
	}
	for _, item := range items {
		if strings.TrimSpace(item.SKU) == "" || strings.TrimSpace(item.Name) == "" || item.Quantity <= 0 || strings.TrimSpace(item.UnitPrice) == "" {
			return fmt.Errorf("%w: each item requires sku, name, positive quantity, and unitPrice", service.ErrInvalid)
		}
	}
	return nil
}

func calculateTotal(items []domain.OrderItem) (string, error) {
	total := new(big.Rat)
	for _, item := range items {
		price, ok := new(big.Rat).SetString(item.UnitPrice)
		if !ok || price.Sign() < 0 {
			return "", fmt.Errorf("%w: unitPrice must be a non-negative decimal", service.ErrInvalid)
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
