package signal

import "gorm.io/gorm"

type Migrator struct{}

func NewMigrator() Migrator {
	return Migrator{}
}

func (Migrator) Name() string {
	return "signal"
}

func (Migrator) AutoMigrate(db *gorm.DB) error {
	return db.AutoMigrate(
		&WebhookEndpointRecord{},
		&SymbolSignalConfigRecord{},
		&SignalEventRecord{},
		&WebhookDeliveryRecord{},
	)
}
