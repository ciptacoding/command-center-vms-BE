.PHONY: run build test clean deps migrate

# Run the application
run:
	go run main.go

# Build the application
build:
	go build -o bin/server main.go

# Run tests
test:
	go test ./...

# Clean build artifacts
clean:
	rm -rf bin/
	rm -rf hls_output/

# Download dependencies
deps:
	go mod download
	go mod tidy

# Run database migrations (auto on startup)
migrate:
	@echo "Migrations run automatically on application startup"

# Install ffmpeg (Ubuntu/Debian)
install-ffmpeg-ubuntu:
	sudo apt-get update
	sudo apt-get install -y ffmpeg

# Install ffmpeg (macOS)
install-ffmpeg-macos:
	brew install ffmpeg

# Check if ffmpeg is installed
check-ffmpeg:
	@which ffmpeg || echo "FFmpeg is not installed. Please install it for RTSP to HLS conversion."

