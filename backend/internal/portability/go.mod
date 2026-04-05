module github.com/chifamba/paperclip/backend/internal/portability

go 1.25.0

require (
	github.com/chifamba/paperclip/backend/db v0.0.0
	gorm.io/datatypes v1.2.7
	gorm.io/gorm v1.31.1
)

replace github.com/chifamba/paperclip/backend/db => ../../db
