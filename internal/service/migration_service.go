package service

import (
	"context"

	"github.com/srjung/debezium-test/internal/domain"
)

type MigrationService interface {
	Create(context.Context) (domain.MigrationRun, error)
	Get(context.Context, string) (domain.MigrationRun, error)
	StartBulk(context.Context, string) (domain.MigrationRun, error)
	StartReplay(context.Context, string) (domain.MigrationRun, error)
	StartValidation(context.Context, string) (domain.MigrationRun, error)
	StartCutover(context.Context, string) (domain.MigrationRun, error)
}
