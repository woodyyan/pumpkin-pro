package feedback

import "time"

type FeedbackRecord struct {
	ID        string    `gorm:"primaryKey;size:36" json:"id"`
	UserID    string    `gorm:"size:36;not null;index" json:"user_id"`
	UserEmail string    `gorm:"size:128;not null;default:''" json:"user_email"`
	Category  string    `gorm:"size:20;not null;index" json:"category"`
	Content   string    `gorm:"type:text;not null" json:"content"`
	Contact   string    `gorm:"size:128;not null;default:''" json:"contact"`
	Status    string    `gorm:"size:20;not null;default:'pending';index" json:"status"`
	CreatedAt time.Time `gorm:"not null;index" json:"created_at"`
	UpdatedAt time.Time `gorm:"not null" json:"updated_at"`
}

func (FeedbackRecord) TableName() string {
	return "feedback_records"
}

type FeedbackStats struct {
	Total        int64 `json:"total"`
	Pending      int64 `json:"pending"`
	BugCount     int64 `json:"bug_count"`
	FeatureCount int64 `json:"feature_count"`
	WishCount    int64 `json:"wish_count"`
}
