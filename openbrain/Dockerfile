# Placeholder Dockerfile for OpenBrain
FROM golang:1.25-alpine AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN go build -o /openbrain ./cmd/openbrain

FROM alpine:latest
COPY --from=builder /openbrain /openbrain
EXPOSE 8080 9090
CMD ["/openbrain"]
