# Command Center VMS CCTV - Backend

Backend service untuk Command Center VMS CCTV menggunakan Go, PostgreSQL, dan RTSP to HLS streaming.

## Features

- 🔐 JWT Authentication
- 📹 Camera Management (CRUD)
- 🎥 RTSP to HLS Stream Conversion
- 🗄️ PostgreSQL Database
- 🚀 RESTful API

## Prerequisites

- Go 1.21 or higher
- PostgreSQL 12 or higher
- FFmpeg (for RTSP to HLS conversion)

## Setup

1. **Install dependencies:**
```bash
go mod download
```

2. **Setup environment variables:**
```bash
cp .env.example .env
# Edit .env with your configuration
```

3. **Setup PostgreSQL database:**
```sql
CREATE DATABASE vms_cctv;
```

4. **Run migrations:**
The application will automatically run migrations on startup.

5. **Start the server:**
```bash
go run main.go
```

## API Endpoints

### Authentication

- `POST /api/v1/auth/login` - Login user
- `GET /api/v1/auth/me` - Get current user (protected)
- `POST /api/v1/auth/logout` - Logout (protected)

### Cameras

- `GET /api/v1/cameras` - Get all cameras (protected)
- `GET /api/v1/cameras/:id` - Get camera by ID (protected)
- `POST /api/v1/cameras` - Create camera (protected)
- `PUT /api/v1/cameras/:id` - Update camera (protected)
- `DELETE /api/v1/cameras/:id` - Delete camera (protected)
- `GET /api/v1/cameras/:id/stream` - Get HLS stream URL (protected)

## Default Credentials

- Email: `admin@vms.demo`
- Password: `demo123`

## RTSP to HLS Conversion

The service uses RTSP to HLS conversion for streaming. Make sure FFmpeg is installed:

```bash
# Ubuntu/Debian
sudo apt-get install ffmpeg

# macOS
brew install ffmpeg
```

## Project Structure

```
BE/
├── config/         # Configuration
├── database/       # Database initialization
├── handlers/       # HTTP handlers
├── middleware/     # Middleware (auth, etc)
├── models/         # Database models
├── services/       # Business logic (RTSP service)
└── utils/          # Utility functions
```

## Development

Run in development mode:
```bash
GIN_MODE=debug go run main.go
```

## Notes

- The RTSP to HLS conversion is currently a placeholder. You'll need to implement the actual conversion using ffmpeg or a Go library like `github.com/deepch/vdk`.
- Make sure to change the JWT_SECRET in production.
- The default admin user is created automatically on first run.

