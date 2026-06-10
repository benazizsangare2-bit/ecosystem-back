FROM golang:1.25.0-alpine AS builder

WORKDIR /app

# Copy go mod files first (for caching)
COPY go.mod go.sum ./
RUN go mod download

# Copy all source code
COPY . .

# Build the binary
RUN CGO_ENABLED=0 GOOS=linux go build -o ecosystem .

FROM alpine:latest

RUN apk --no-cache add ca-certificates

WORKDIR /app
COPY --from=builder /app/ecosystem .
COPY --from=builder /app/.env .
COPY --from=builder /app/docs ./docs

EXPOSE 3030

CMD ["./ecosystem"]