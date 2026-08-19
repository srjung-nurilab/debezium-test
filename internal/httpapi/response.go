package httpapi

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/srjung/debezium-test/internal/service"
)

func writeError(c *gin.Context, err error) {
	status := http.StatusInternalServerError
	code := "INTERNAL"

	switch {
	case errors.Is(err, service.ErrInvalid):
		status = http.StatusBadRequest
		code = "INVALID_ARGUMENT"
	case errors.Is(err, service.ErrNotFound):
		status = http.StatusNotFound
		code = "NOT_FOUND"
	case errors.Is(err, service.ErrConflict):
		status = http.StatusConflict
		code = "CONFLICT"
	}

	c.JSON(status, gin.H{"error": gin.H{"code": code, "message": err.Error()}})
}

func idempotencyKey(c *gin.Context) (string, bool) {
	key := c.GetHeader("Idempotency-Key")
	if key == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"code": "INVALID_ARGUMENT", "message": "Idempotency-Key header is required"}})
		return "", false
	}
	return key, true
}
