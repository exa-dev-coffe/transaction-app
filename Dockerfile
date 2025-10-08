# Stage 1: Build Go binary
FROM golang:1.24.5-alpine AS builder

RUN apk add --no-cache git build-base

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN go build -o master-data .

# Stage 2: Runtime
FROM alpine:3.19

RUN apk add --no-cache ca-certificates

WORKDIR /app
COPY --from=builder /app/master-data .
COPY db/migrations ./db/migrations

EXPOSE 8001
CMD ["./master-data"]
