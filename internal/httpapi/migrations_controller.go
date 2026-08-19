package httpapi

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/srjung/debezium-test/internal/domain"
	"github.com/srjung/debezium-test/internal/service"
)

type MigrationController struct {
	service service.MigrationService
}

func NewMigrationController(service service.MigrationService) *MigrationController {
	return &MigrationController{service: service}
}

func (controller *MigrationController) Create(c *gin.Context) {
	run, err := controller.service.Create(c.Request.Context())
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusCreated, run)
}

func (controller *MigrationController) Get(c *gin.Context) {
	run, err := controller.service.Get(c.Request.Context(), c.Param("id"))
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, run)
}

func (controller *MigrationController) StartBulk(c *gin.Context) {
	controller.transition(c, controller.service.StartBulk)
}

func (controller *MigrationController) StartReplay(c *gin.Context) {
	controller.transition(c, controller.service.StartReplay)
}

func (controller *MigrationController) StartValidation(c *gin.Context) {
	controller.transition(c, controller.service.StartValidation)
}

func (controller *MigrationController) StartCutover(c *gin.Context) {
	controller.transition(c, controller.service.StartCutover)
}

func (controller *MigrationController) transition(c *gin.Context, action func(context.Context, string) (domain.MigrationRun, error)) {
	run, err := action(c.Request.Context(), c.Param("id"))
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusAccepted, run)
}
