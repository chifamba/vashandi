FROM golang:1.25-alpine AS build

RUN apk add --no-cache git gcc musl-dev

WORKDIR /app

# Copy all go modules first
COPY vashandi/backend/shared/go.mod vashandi/backend/shared/go.sum vashandi/backend/shared/
COPY vashandi/backend/db/go.mod vashandi/backend/db/go.sum vashandi/backend/db/
COPY vashandi/backend/server/go.mod vashandi/backend/server/go.sum vashandi/backend/server/
COPY vashandi/backend/cmd/paperclipai/go.mod vashandi/backend/cmd/paperclipai/go.sum vashandi/backend/cmd/paperclipai/

# Go workspace
COPY vashandi/go.work vashandi/go.work.sum vashandi/

WORKDIR /app/vashandi

# Download dependencies
RUN go mod download

# Copy source code
COPY vashandi/backend/shared /app/vashandi/backend/shared
COPY vashandi/backend/db /app/vashandi/backend/db
COPY vashandi/backend/server /app/vashandi/backend/server
COPY vashandi/backend/cmd/paperclipai /app/vashandi/backend/cmd/paperclipai

# Build the CLI (which includes the server 'run' command)
RUN go build -o /app/paperclipai ./backend/cmd/paperclipai

FROM alpine:latest

RUN apk add --no-cache ca-certificates gosu curl

# Copy the binary
COPY --from=build /app/paperclipai /usr/local/bin/paperclipai

# Default environment variables
ENV PAPERCLIP_DEPLOYMENT_MODE=authenticated \
    PAPERCLIP_DEPLOYMENT_EXPOSURE=private \
    PORT=3100

VOLUME ["/paperclip"]
EXPOSE 3100

# Entrypoint to handle permissions if needed (simple version)
COPY vashandi/scripts/docker-entrypoint.sh /usr/local/bin/docker-entrypoint.sh
RUN chmod +x /usr/local/bin/docker-entrypoint.sh

# Use original entrypoint logic if it's compatible
ENTRYPOINT ["docker-entrypoint.sh"]
CMD ["paperclipai", "run"]
