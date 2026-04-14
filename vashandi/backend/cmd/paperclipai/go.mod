module github.com/chifamba/vashandi/vashandi/backend/cmd/paperclipai

go 1.25.0

require (
	github.com/chifamba/vashandi/vashandi/backend/adapters v0.0.0-00010101000000-000000000000
	github.com/chifamba/vashandi/vashandi/backend/client v0.0.0-00010101000000-000000000000
	github.com/spf13/cobra v1.10.2
)

require (
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/spf13/pflag v1.0.9 // indirect
)

replace (
	github.com/chifamba/vashandi/vashandi/backend/adapters => ../../adapters
	github.com/chifamba/vashandi/vashandi/backend/client => ../../client
)
