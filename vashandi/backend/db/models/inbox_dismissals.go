package models

import "time"

type InboxDismissal struct {
	ID          string    `gorm:"column:id;type:uuid;primaryKey;default:gen_random_uuid()"`
	CompanyID   string    `gorm:"column:company_id;type:uuid;not null;index:inbox_dismissals_company_user_idx;index:inbox_dismissals_company_item_idx;uniqueIndex:inbox_dismissals_company_user_item_idx"`
	UserID      string    `gorm:"column:user_id;not null;index:inbox_dismissals_company_user_idx;uniqueIndex:inbox_dismissals_company_user_item_idx"`
	ItemKey     string    `gorm:"column:item_key;not null;index:inbox_dismissals_company_item_idx;uniqueIndex:inbox_dismissals_company_user_item_idx"`
	DismissedAt time.Time `gorm:"column:dismissed_at;type:timestamptz;not null;default:now()"`
	CreatedAt   time.Time `gorm:"column:created_at;type:timestamptz;not null;default:now()"`
	UpdatedAt   time.Time `gorm:"column:updated_at;type:timestamptz;not null;default:now()"`

	Company Company `gorm:"foreignKey:CompanyID"`
}

func (InboxDismissal) TableName() string {
	return "inbox_dismissals"
}
