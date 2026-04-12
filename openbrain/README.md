# OpenBrain

OpenBrain is the dedicated memory service for Vashandi agents.

## Features

- company-scoped namespaces and registered agents
- rich memory entities with tiers, provenance, identity, versions, and relationship edges
- semantic + lexical search and context compilation with token budgets
- promotion, decay, curator proposals, audit export, and pending context packets
- REST, gRPC, and MCP interfaces
- CLI support for memory management, audit export, health checks, scoped token generation, and repository sync polling

## Running locally

```sh
cd openbrain
go test ./...
go build ./cmd/openbrain
./openbrain serve
```

Environment variables:

- `DATABASE_URL` — PostgreSQL DSN (defaults to `postgres://paperclip:paperclip@localhost:5432/paperclip?sslmode=disable`)
- `PORT` — HTTP port (default `3101`)
- `GRPC_PORT` — gRPC port (default `50051`)
- `OPENBRAIN_API_KEY` — legacy bearer token secret
- `OPENBRAIN_SIGNING_SECRET` — HMAC secret for scoped tokens

## Key HTTP endpoints

- `POST /api/v1/memories`
- `GET /api/v1/memories`
- `POST /api/v1/memories/search`
- `POST /api/v1/context/compile`
- `GET /api/v1/context/pending`
- `GET /api/v1/audit/export`
- `POST /internal/v1/namespaces/:namespaceId/agents`
- `POST /internal/v1/namespaces/:namespaceId/triggers/:triggerType`

Legacy `/v1/namespaces/:namespaceId/...` routes remain available for Vashandi compatibility.
