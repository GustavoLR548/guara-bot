# Build stage
FROM golang:1.23-alpine AS builder

WORKDIR /app

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build for the target architecture
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o godot-news-bot ./cmd/bot

# Runtime stage
FROM alpine:latest

RUN apk --no-cache add ca-certificates tzdata

WORKDIR /root/

# Copy the binary from builder
COPY --from=builder /app/godot-news-bot .

# Run the bot
CMD ["./godot-news-bot"]