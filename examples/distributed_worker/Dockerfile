FROM golang:1.24-alpine AS builder

WORKDIR /app

# Copy go.mod and go.sum
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy the source code
COPY . .

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -o worker-app ./examples/distributed_worker/main.go

# Use a minimal alpine image for the final container
FROM alpine:latest

WORKDIR /app

# Install necessary runtime dependencies
RUN apk --no-cache add ca-certificates

# Copy the compiled binary from the builder stage
COPY --from=builder /app/worker-app .

# Copy any configuration files
COPY examples/distributed_worker/config ./config

# Run the distributed worker application
ENTRYPOINT ["/app/worker-app"] 