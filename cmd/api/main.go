package main

import (
	"context"
	"log"
	"os"
	"time"

	"github.com/srjung/debezium-test/internal/httpapi"
	"github.com/srjung/debezium-test/internal/repository/mongodb"
	"github.com/srjung/debezium-test/internal/service"
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	mongoURI := os.Getenv("MONGODB_URI")
	if mongoURI == "" {
		mongoURI = "mongodb://localhost:27017/?directConnection=true"
	}
	mongoDatabase := os.Getenv("MONGODB_DATABASE")
	if mongoDatabase == "" {
		mongoDatabase = "app"
	}

	connectContext, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	orders, mongoClient, err := mongodb.Connect(connectContext, mongoURI, mongoDatabase)
	if err != nil {
		log.Fatal(err)
	}
	defer func() {
		if err := mongoClient.Disconnect(context.Background()); err != nil {
			log.Printf("disconnect MongoDB: %v", err)
		}
	}()

	router := httpapi.NewRouter(
		orders,
		service.NewMemoryMigrationService(),
	)

	if err := router.Run(":" + port); err != nil {
		log.Fatal(err)
	}
}
