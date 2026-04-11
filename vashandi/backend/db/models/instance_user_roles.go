package models

import (
	"time"
)

type InstanceUserRole struct {
	ID        string    `gorm:"column:id;type:uuid;primaryKey;default:gen_random_uuid()"`
	UserID    string    `gorm:"column:user_id;not null;uniqueIndex:instance_user_roles_user_role_unique_idx"`
	Role      string    `gorm:"column:role;not null;default:instance_admin;uniqueIndex:instance_user_roles_user_role_unique_idx;index:instance_user_roles_role_idx"`
	CreatedAt time.Time `gorm:"column:created_at;type:timestamptz;not null;default:now()"`
	UpdatedAt time.Time `gorm:"column:updated_at;type:timestamptz;not null;default:now()"`

	User User `gorm:"foreignKey:UserID"`
}

func (InstanceUserRole) TableName() string {
	return "instance_user_roles"
}
