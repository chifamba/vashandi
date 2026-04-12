FROM golang:1.25-alpine AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN go build -o /openbrain ./cmd/openbrain

FROM alpine:latest
RUN adduser -D -u 10001 openbrain
COPY --from=builder /openbrain /usr/local/bin/openbrain
USER openbrain
EXPOSE 3101 50051
CMD ["/usr/local/bin/openbrain"]
