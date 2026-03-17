package auth

import "gorm.io/gorm"

type Migrator struct{}

func NewMigrator() Migrator {
	return Migrator{}
}

func (Migrator) Name() string {
	return "auth"
}

func (Migrator) AutoMigrate(db *gorm.DB) error {
	return db.AutoMigrate(
		&UserRecord{},
		&UserProfileRecord{},
		&UserSessionRecord{},
		&AuthAuditRecord{},
	)
}
