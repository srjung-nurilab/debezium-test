package domain

import "time"

type MigrationState string

const (
	MigrationStateMongoPrimary     MigrationState = "MONGO_PRIMARY"
	MigrationStateCDCBuffering     MigrationState = "CDC_BUFFERING"
	MigrationStateBulkLoading      MigrationState = "BULK_LOADING"
	MigrationStateCDCReplaying     MigrationState = "CDC_REPLAYING"
	MigrationStateShadowValidating MigrationState = "SHADOW_VALIDATING"
	MigrationStateCutoverQueuing   MigrationState = "CUTOVER_QUEUING"
	MigrationStatePostgresPrimary  MigrationState = "POSTGRES_PRIMARY"
	MigrationStateFinalized        MigrationState = "FINALIZED"
)

type MigrationRun struct {
	ID        string         `json:"id"`
	State     MigrationState `json:"state"`
	CreatedAt time.Time      `json:"createdAt"`
	UpdatedAt time.Time      `json:"updatedAt"`
}
