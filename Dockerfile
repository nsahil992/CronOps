# Stage 1: Building the Go binary
FROM golang:1.23.1-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN go build -o cronops

# Stage 2: Multi-stage build

FROM alpine:latest

WORKDIR /app

COPY --from=builder /app/cronops .
COPY --from=builder /app/static ./static
COPY --from=builder /app/.env .env

EXPOSE 8080

CMD ["./cronops"]


