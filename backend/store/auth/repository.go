package auth

import (
	"context"
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"
)

type Repository struct {
	db *gorm.DB
}

func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

func (r *Repository) CreateUser(ctx context.Context, user UserRecord, profile UserProfileRecord) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&user).Error; err != nil {
			if isUniqueError(err) {
				return ErrEmailAlreadyExists
			}
			return err
		}
		if err := tx.Create(&profile).Error; err != nil {
			return err
		}
		return nil
	})
}

func (r *Repository) GetUserByEmail(ctx context.Context, email string) (*UserRecord, error) {
	var record UserRecord
	if err := r.db.WithContext(ctx).First(&record, "email = ?", email).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrUserNotFound
		}
		return nil, err
	}
	return &record, nil
}

func (r *Repository) GetUserByID(ctx context.Context, userID string) (*UserRecord, error) {
	var record UserRecord
	if err := r.db.WithContext(ctx).First(&record, "id = ?", userID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrUserNotFound
		}
		return nil, err
	}
	return &record, nil
}

func (r *Repository) GetProfileByUserID(ctx context.Context, userID string) (*UserProfileRecord, error) {
	var record UserProfileRecord
	if err := r.db.WithContext(ctx).First(&record, "user_id = ?", userID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrUserNotFound
		}
		return nil, err
	}
	return &record, nil
}

func (r *Repository) UpsertProfile(ctx context.Context, profile UserProfileRecord) error {
	profile.UpdatedAt = time.Now().UTC()
	return r.db.WithContext(ctx).
		Model(&UserProfileRecord{}).
		Where("user_id = ?", profile.UserID).
		Assign(profile).
		FirstOrCreate(&UserProfileRecord{UserID: profile.UserID}).Error
}

func (r *Repository) UpdateUserEmail(ctx context.Context, userID string, email string) error {
	result := r.db.WithContext(ctx).Model(&UserRecord{}).Where("id = ?", userID).Updates(map[string]any{
		"email":      email,
		"updated_at": time.Now().UTC(),
	})
	if result.Error != nil {
		if isUniqueError(result.Error) {
			return ErrEmailAlreadyExists
		}
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrUserNotFound
	}
	return nil
}

func (r *Repository) UpdatePasswordHash(ctx context.Context, userID string, hash string) error {
	result := r.db.WithContext(ctx).Model(&UserRecord{}).Where("id = ?", userID).Updates(map[string]any{
		"password_hash": hash,
		"updated_at":    time.Now().UTC(),
	})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrUserNotFound
	}
	return nil
}

func (r *Repository) TouchLastLogin(ctx context.Context, userID string) error {
	return r.db.WithContext(ctx).Model(&UserRecord{}).Where("id = ?", userID).Update("updated_at", time.Now().UTC()).Error
}

func (r *Repository) CreateSession(ctx context.Context, session UserSessionRecord) error {
	return r.db.WithContext(ctx).Create(&session).Error
}

func (r *Repository) GetSessionByRefreshHash(ctx context.Context, hash string) (*UserSessionRecord, error) {
	var record UserSessionRecord
	if err := r.db.WithContext(ctx).First(&record, "refresh_token_hash = ?", hash).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrSessionNotFound
		}
		return nil, err
	}
	return &record, nil
}

func (r *Repository) RevokeSessionByID(ctx context.Context, sessionID string) error {
	now := time.Now().UTC()
	return r.db.WithContext(ctx).Model(&UserSessionRecord{}).Where("id = ?", sessionID).Update("revoked_at", &now).Error
}

func (r *Repository) RevokeSessionByHash(ctx context.Context, hash string) error {
	now := time.Now().UTC()
	return r.db.WithContext(ctx).Model(&UserSessionRecord{}).Where("refresh_token_hash = ?", hash).Update("revoked_at", &now).Error
}

func (r *Repository) InsertAudit(ctx context.Context, record AuthAuditRecord) {
	_ = r.db.WithContext(ctx).Create(&record).Error
}

func isUniqueError(err error) bool {
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return true
	}
	text := strings.ToLower(strings.TrimSpace(err.Error()))
	return strings.Contains(text, "unique") || strings.Contains(text, "duplicate")
}
