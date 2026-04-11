package models

import (
	"time"
)

type User struct {
	ID            string    `gorm:"column:id;primaryKey"`
	Name          string    `gorm:"column:name;not null"`
	Email         string    `gorm:"column:email;not null"`
	EmailVerified bool      `gorm:"column:email_verified;not null;default:false"`
	Image         *string   `gorm:"column:image"`
	CreatedAt     time.Time `gorm:"column:created_at;not null;type:timestamptz"`
	UpdatedAt     time.Time `gorm:"column:updated_at;not null;type:timestamptz"`
}

func (User) TableName() string {
	return "user"
}

type Session struct {
	ID        string    `gorm:"column:id;primaryKey"`
	ExpiresAt time.Time `gorm:"column:expires_at;not null;type:timestamptz"`
	Token     string    `gorm:"column:token;not null"`
	CreatedAt time.Time `gorm:"column:created_at;not null;type:timestamptz"`
	UpdatedAt time.Time `gorm:"column:updated_at;not null;type:timestamptz"`
	IPAddress *string   `gorm:"column:ip_address"`
	UserAgent *string   `gorm:"column:user_agent"`
	UserID    string    `gorm:"column:user_id;not null"`
	User      User      `gorm:"foreignKey:UserID;constraint:OnDelete:CASCADE"`
}

func (Session) TableName() string {
	return "session"
}

type Account struct {
	ID                    string     `gorm:"column:id;primaryKey"`
	AccountID             string     `gorm:"column:account_id;not null"`
	ProviderID            string     `gorm:"column:provider_id;not null"`
	UserID                string     `gorm:"column:user_id;not null"`
	AccessToken           *string    `gorm:"column:access_token"`
	RefreshToken          *string    `gorm:"column:refresh_token"`
	IDToken               *string    `gorm:"column:id_token"`
	AccessTokenExpiresAt  *time.Time `gorm:"column:access_token_expires_at;type:timestamptz"`
	RefreshTokenExpiresAt *time.Time `gorm:"column:refresh_token_expires_at;type:timestamptz"`
	Scope                 *string    `gorm:"column:scope"`
	Password              *string    `gorm:"column:password"`
	CreatedAt             time.Time  `gorm:"column:created_at;not null;type:timestamptz"`
	UpdatedAt             time.Time  `gorm:"column:updated_at;not null;type:timestamptz"`
	User                  User       `gorm:"foreignKey:UserID;constraint:OnDelete:CASCADE"`
}

func (Account) TableName() string {
	return "account"
}

type Verification struct {
	ID         string     `gorm:"column:id;primaryKey"`
	Identifier string     `gorm:"column:identifier;not null"`
	Value      string     `gorm:"column:value;not null"`
	ExpiresAt  time.Time  `gorm:"column:expires_at;not null;type:timestamptz"`
	CreatedAt  *time.Time `gorm:"column:created_at;type:timestamptz"`
	UpdatedAt  *time.Time `gorm:"column:updated_at;type:timestamptz"`
}

func (Verification) TableName() string {
	return "verification"
}
