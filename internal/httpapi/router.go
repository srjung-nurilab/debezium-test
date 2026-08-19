package httpapi

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/srjung/debezium-test/internal/service"
)

func NewRouter(orderService service.OrderService, migrationService service.MigrationService) *gin.Engine {
	router := gin.New()
	router.Use(gin.Logger(), gin.Recovery())

	router.GET("/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	orders := NewOrderController(orderService)
	router.POST("/orders", orders.Create)
	router.GET("/orders", orders.List)
	router.GET("/orders/:id", orders.Get)
	router.PUT("/orders/:id", orders.Update)
	router.DELETE("/orders/:id", orders.Delete)

	migrations := NewMigrationController(migrationService)
	admin := router.Group("/admin/migrations")
	admin.POST("", migrations.Create)
	admin.GET("/:id", migrations.Get)
	admin.POST("/:id/bulk", migrations.StartBulk)
	admin.POST("/:id/replay", migrations.StartReplay)
	admin.POST("/:id/validate", migrations.StartValidation)
	admin.POST("/:id/cutover", migrations.StartCutover)

	return router
}
