package feedback

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

var allowedCategories = map[string]bool{
	"bug":     true,
	"feature": true,
	"wish":    true,
}

var allowedStatuses = map[string]bool{
	"pending":   true,
	"resolved":  true,
	"dismissed": true,
}

type Repository struct {
	db *gorm.DB
}

func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

type CreateInput struct {
	UserID    string
	UserEmail string
	Category  string
	Content   string
	Contact   string
}

func (r *Repository) Create(ctx context.Context, input CreateInput) (*FeedbackRecord, error) {
	category := strings.ToLower(strings.TrimSpace(input.Category))
	if !allowedCategories[category] {
		return nil, fmt.Errorf("%w: 不支持的反馈类型: %s", ErrInvalid, category)
	}

	content := strings.TrimSpace(input.Content)
	if content == "" {
		return nil, fmt.Errorf("%w: 反馈内容不能为空", ErrInvalid)
	}
	if len([]rune(content)) > 2000 {
		return nil, fmt.Errorf("%w: 反馈内容不能超过 2000 字", ErrInvalid)
	}

	contact := strings.TrimSpace(input.Contact)
	if len([]rune(contact)) > 128 {
		contact = string([]rune(contact)[:128])
	}

	now := time.Now().UTC()
	record := FeedbackRecord{
		ID:        uuid.NewString(),
		UserID:    strings.TrimSpace(input.UserID),
		UserEmail: strings.TrimSpace(input.UserEmail),
		Category:  category,
		Content:   content,
		Contact:   contact,
		Status:    "pending",
		CreatedAt: now,
		UpdatedAt: now,
	}

	if err := r.db.WithContext(ctx).Create(&record).Error; err != nil {
		return nil, err
	}
	return &record, nil
}

func (r *Repository) List(ctx context.Context, limit, offset int) ([]FeedbackRecord, int64, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}
	if offset < 0 {
		offset = 0
	}

	var total int64
	if err := r.db.WithContext(ctx).Model(&FeedbackRecord{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var items []FeedbackRecord
	if err := r.db.WithContext(ctx).
		Order("created_at DESC").
		Limit(limit).Offset(offset).
		Find(&items).Error; err != nil {
		return nil, 0, err
	}

	return items, total, nil
}

func (r *Repository) UpdateStatus(ctx context.Context, id string, status string) error {
	status = strings.ToLower(strings.TrimSpace(status))
	if !allowedStatuses[status] {
		return fmt.Errorf("%w: 不支持的状态: %s", ErrInvalid, status)
	}

	result := r.db.WithContext(ctx).
		Model(&FeedbackRecord{}).
		Where("id = ?", strings.TrimSpace(id)).
		Updates(map[string]any{
			"status":     status,
			"updated_at": time.Now().UTC(),
		})

	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *Repository) GetStats(ctx context.Context) (*FeedbackStats, error) {
	var total, pending, bugCount, featureCount, wishCount int64

	r.db.WithContext(ctx).Model(&FeedbackRecord{}).Count(&total)
	r.db.WithContext(ctx).Model(&FeedbackRecord{}).Where("status = ?", "pending").Count(&pending)
	r.db.WithContext(ctx).Model(&FeedbackRecord{}).Where("category = ?", "bug").Count(&bugCount)
	r.db.WithContext(ctx).Model(&FeedbackRecord{}).Where("category = ?", "feature").Count(&featureCount)
	r.db.WithContext(ctx).Model(&FeedbackRecord{}).Where("category = ?", "wish").Count(&wishCount)

	return &FeedbackStats{
		Total:        total,
		Pending:      pending,
		BugCount:     bugCount,
		FeatureCount: featureCount,
		WishCount:    wishCount,
	}, nil
}

func (r *Repository) GetByID(ctx context.Context, id string) (*FeedbackRecord, error) {
	var record FeedbackRecord
	if err := r.db.WithContext(ctx).First(&record, "id = ?", strings.TrimSpace(id)).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &record, nil
}
