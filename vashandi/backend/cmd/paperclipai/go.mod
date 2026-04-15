module github.com/chifamba/vashandi/vashandi/backend/cmd/paperclipai

go 1.25.0

require (
	github.com/chifamba/vashandi/vashandi/backend/adapters v0.0.0-00010101000000-000000000000
	github.com/chifamba/vashandi/vashandi/backend/client v0.0.0-00010101000000-000000000000
	github.com/chifamba/vashandi/vashandi/backend/db v0.0.0-20260414042850-ccad1bd164ff
	github.com/chifamba/vashandi/vashandi/backend/server v0.0.0-00010101000000-000000000000
	github.com/spf13/cobra v1.10.2
	gorm.io/driver/postgres v1.6.0
	gorm.io/gorm v1.31.1
)

require (
	filippo.io/edwards25519 v1.1.1 // indirect
	github.com/chifamba/vashandi/vashandi/backend/shared v0.0.0-20260414011212-34c73c8a303c // indirect
	github.com/go-chi/chi/v5 v5.2.5 // indirect
	github.com/go-chi/cors v1.2.2 // indirect
	github.com/go-sql-driver/mysql v1.8.1 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/gorilla/websocket v1.5.3 // indirect
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/jackc/pgpassfile v1.0.0 // indirect
	github.com/jackc/pgservicefile v0.0.0-20240606120523-5a60cdf6a761 // indirect
	github.com/jackc/pgx/v5 v5.7.2 // indirect
	github.com/jackc/puddle/v2 v2.2.2 // indirect
	github.com/jinzhu/inflection v1.0.0 // indirect
	github.com/jinzhu/now v1.1.5 // indirect
	github.com/lib/pq v1.12.3 // indirect
	github.com/spf13/pflag v1.0.9 // indirect
	golang.org/x/crypto v0.50.0 // indirect
	golang.org/x/net v0.52.0 // indirect
	golang.org/x/sync v0.20.0 // indirect
	golang.org/x/text v0.36.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
	gorm.io/datatypes v1.2.7 // indirect
	gorm.io/driver/mysql v1.5.6 // indirect
)

replace (
	github.com/chifamba/vashandi/vashandi/backend/adapters => ../../adapters
	github.com/chifamba/vashandi/vashandi/backend/client => ../../client
	github.com/chifamba/vashandi/vashandi/backend/server => ../../server
)
