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

func (r *Repository) UpdatePasswordHashAndBumpCredentialVersion(ctx context.Context, userID string, hash string) error {
	result := r.db.WithContext(ctx).Model(&UserRecord{}).Where("id = ?", userID).Updates(map[string]any{
		"password_hash":      hash,
		"credential_version": gorm.Expr("credential_version + 1"),
		"updated_at":         time.Now().UTC(),
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

func (r *Repository) RevokeAllSessionsByUserID(ctx context.Context, userID string) error {
	now := time.Now().UTC()
	return r.db.WithContext(ctx).Model(&UserSessionRecord{}).
		Where("user_id = ? AND revoked_at IS NULL", userID).
		Update("revoked_at", &now).Error
}

func (r *Repository) CreatePasswordResetToken(ctx context.Context, record PasswordResetTokenRecord) error {
	return r.db.WithContext(ctx).Create(&record).Error
}

func (r *Repository) DeletePasswordResetTokenByID(ctx context.Context, id string) error {
	return r.db.WithContext(ctx).Delete(&PasswordResetTokenRecord{}, "id = ?", id).Error
}

func (r *Repository) GetPasswordResetTokenByHash(ctx context.Context, tokenHash string) (*PasswordResetTokenRecord, error) {
	var record PasswordResetTokenRecord
	if err := r.db.WithContext(ctx).First(&record, "token_hash = ?", tokenHash).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrResetTokenNotFound
		}
		return nil, err
	}
	return &record, nil
}

func (r *Repository) CreatePasswordResetAttempt(ctx context.Context, record PasswordResetAttemptRecord) error {
	return r.db.WithContext(ctx).Create(&record).Error
}

func (r *Repository) DeletePasswordResetAttemptByID(ctx context.Context, id string) error {
	return r.db.WithContext(ctx).Delete(&PasswordResetAttemptRecord{}, "id = ?", id).Error
}

func (r *Repository) CountPasswordResetAttemptsByEmailSince(ctx context.Context, email string, since time.Time) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&PasswordResetAttemptRecord{}).
		Where("email = ? AND created_at >= ?", email, since).
		Count(&count).Error
	return count, err
}

func (r *Repository) CountPasswordResetAttemptsByIPSince(ctx context.Context, ip string, since time.Time) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&PasswordResetAttemptRecord{}).
		Where("ip = ? AND created_at >= ?", ip, since).
		Count(&count).Error
	return count, err
}

func (r *Repository) GetLatestPasswordResetAttemptByEmail(ctx context.Context, email string) (*PasswordResetAttemptRecord, error) {
	var record PasswordResetAttemptRecord
	if err := r.db.WithContext(ctx).Order("created_at DESC").First(&record, "email = ?", email).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &record, nil
}

func (r *Repository) GetOldestPasswordResetAttemptByEmailSince(ctx context.Context, email string, since time.Time) (*PasswordResetAttemptRecord, error) {
	var record PasswordResetAttemptRecord
	if err := r.db.WithContext(ctx).
		Order("created_at ASC").
		First(&record, "email = ? AND created_at >= ?", email, since).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &record, nil
}

func (r *Repository) GetOldestPasswordResetAttemptByIPSince(ctx context.Context, ip string, since time.Time) (*PasswordResetAttemptRecord, error) {
	var record PasswordResetAttemptRecord
	if err := r.db.WithContext(ctx).
		Order("created_at ASC").
		First(&record, "ip = ? AND created_at >= ?", ip, since).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &record, nil
}

func (r *Repository) DeleteActivePasswordResetTokensByUserID(ctx context.Context, userID string) error {
	return r.db.WithContext(ctx).
		Where("user_id = ? AND consumed_at IS NULL", userID).
		Delete(&PasswordResetTokenRecord{}).Error
}

func (r *Repository) ConsumePasswordResetTokenAndResetPassword(ctx context.Context, tokenID, userID, passwordHash string, now time.Time) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&PasswordResetTokenRecord{}).
			Where("id = ? AND user_id = ? AND consumed_at IS NULL AND expires_at > ?", tokenID, userID, now).
			Update("consumed_at", &now)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			var token PasswordResetTokenRecord
			if err := tx.First(&token, "id = ?", tokenID).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return ErrResetTokenNotFound
				}
				return err
			}
			if token.ConsumedAt != nil {
				return ErrResetTokenConsumed
			}
			if !token.ExpiresAt.After(now) {
				return ErrResetTokenExpired
			}
			return ErrResetTokenNotFound
		}

		userResult := tx.Model(&UserRecord{}).Where("id = ?", userID).Updates(map[string]any{
			"password_hash":      passwordHash,
			"credential_version": gorm.Expr("credential_version + 1"),
			"updated_at":         now,
		})
		if userResult.Error != nil {
			return userResult.Error
		}
		if userResult.RowsAffected == 0 {
			return ErrUserNotFound
		}

		if err := tx.Model(&UserSessionRecord{}).
			Where("user_id = ? AND revoked_at IS NULL", userID).
			Update("revoked_at", &now).Error; err != nil {
			return err
		}

		return nil
	})
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
