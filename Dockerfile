FROM golang:1.21-alpine AS builder

WORKDIR /app

# Install build dependencies
RUN apk add --no-cache git

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o server main.go

# Final stage
FROM alpine:latest

# Install ffmpeg with libvpx (VP8/VP9 codec support) for RTSP to WebRTC conversion
# libvpx is included in ffmpeg package for Alpine
RUN apk add --no-cache ffmpeg

WORKDIR /app

# Copy binary from builder
COPY --from=builder /app/server .

# Note: HLS output directory will be created in tmpfs (RAM disk) at runtime
# No need to create it in Dockerfile

EXPOSE 8080

CMD ["./server"]

