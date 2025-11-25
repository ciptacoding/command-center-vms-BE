package handlers

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"

	"command-center-vms-cctv/be/models"
	"command-center-vms-cctv/be/services"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"gorm.io/gorm"
)

type CameraHandler struct {
	db              *gorm.DB
	mediamtxService *services.MediaMTXService
	rtspService     *services.RTSPService
	mjpegService    *services.MJPEGService
	webrtcService   *services.WebRTCService
}

func NewCameraHandler(db *gorm.DB, mediamtxService *services.MediaMTXService, rtspService *services.RTSPService, mjpegService *services.MJPEGService, webrtcService *services.WebRTCService) *CameraHandler {
	return &CameraHandler{
		db:              db,
		mediamtxService: mediamtxService,
		rtspService:     rtspService,
		mjpegService:    mjpegService,
		webrtcService:   webrtcService,
	}
}

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		// Allow all origins for development
		origin := r.Header.Get("Origin")
		if origin == "" {
			return true
		}
		// Allow localhost origins
		return origin == "http://localhost:8080" ||
			origin == "http://localhost:5173" ||
			origin == "http://localhost:3000" ||
			origin == "http://127.0.0.1:8080" ||
			origin == "http://127.0.0.1:5173" ||
			origin == "http://127.0.0.1:3000" ||
			true // Allow all for now
	},
	// Enable compression
	EnableCompression: true,
}

type CreateCameraRequest struct {
	Name      string  `json:"name" binding:"required"`
	Latitude  float64 `json:"latitude" binding:"required"`
	Longitude float64 `json:"longitude" binding:"required"`
	RTSPUrl   string  `json:"rtsp_url" binding:"required"`
	Area      string  `json:"area" binding:"required"`
	Building  string  `json:"building" binding:"required"`
	Status    string  `json:"status"`
}

type UpdateCameraRequest struct {
	Name      *string  `json:"name"`
	Latitude  *float64 `json:"latitude"`
	Longitude *float64 `json:"longitude"`
	RTSPUrl   *string  `json:"rtsp_url"`
	Area      *string  `json:"area"`
	Building  *string  `json:"building"`
	Status    *string  `json:"status"`
}

func (h *CameraHandler) GetCameras(c *gin.Context) {
	var cameras []models.Camera
	if err := h.db.Find(&cameras).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch cameras"})
		return
	}

	c.JSON(http.StatusOK, cameras)
}

func (h *CameraHandler) GetCamera(c *gin.Context) {
	id := c.Param("id")

	var camera models.Camera
	if err := h.db.First(&camera, id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "Camera not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch camera"})
		return
	}

	c.JSON(http.StatusOK, camera)
}

func (h *CameraHandler) CreateCamera(c *gin.Context) {
	var req CreateCameraRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	status := req.Status
	if status == "" {
		status = "offline"
	}

	camera := models.Camera{
		Name:      req.Name,
		Latitude:  req.Latitude,
		Longitude: req.Longitude,
		RTSPUrl:   req.RTSPUrl,
		Status:    status,
		Area:      req.Area,
		Building:  req.Building,
	}

	if err := h.db.Create(&camera).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create camera"})
		return
	}

	c.JSON(http.StatusCreated, camera)
}

func (h *CameraHandler) UpdateCamera(c *gin.Context) {
	id := c.Param("id")

	var req UpdateCameraRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var camera models.Camera
	if err := h.db.First(&camera, id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "Camera not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch camera"})
		return
	}

	// Update fields if provided
	if req.Name != nil {
		camera.Name = *req.Name
	}
	if req.Latitude != nil {
		camera.Latitude = *req.Latitude
	}
	if req.Longitude != nil {
		camera.Longitude = *req.Longitude
	}
	if req.RTSPUrl != nil {
		camera.RTSPUrl = *req.RTSPUrl
	}
	if req.Area != nil {
		camera.Area = *req.Area
	}
	if req.Building != nil {
		camera.Building = *req.Building
	}
	if req.Status != nil {
		camera.Status = *req.Status
	}

	if err := h.db.Save(&camera).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update camera"})
		return
	}

	c.JSON(http.StatusOK, camera)
}

func (h *CameraHandler) DeleteCamera(c *gin.Context) {
	id := c.Param("id")

	if err := h.db.Delete(&models.Camera{}, id).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete camera"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Camera deleted successfully"})
}

func (h *CameraHandler) GetStreamURL(c *gin.Context) {
	id := c.Param("id")

	var camera models.Camera
	if err := h.db.First(&camera, id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "Camera not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch camera"})
		return
	}

	// Configure MediaMTX path and get HLS URL
	// MediaMTX will pull RTSP stream from camera and serve as HLS
	hlsURL, err := h.mediamtxService.StartStream(camera.ID, camera.RTSPUrl)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to configure MediaMTX stream: " + err.Error()})
		return
	}

	// Get stream health status
	isHealthy, _ := h.mediamtxService.GetStreamHealth(camera.ID)

	c.JSON(http.StatusOK, gin.H{
		"hls_url":    hlsURL,
		"camera_id":  camera.ID,
		"is_healthy": isHealthy,
	})
}

func (h *CameraHandler) GetStreamHealth(c *gin.Context) {
	id := c.Param("id")

	var camera models.Camera
	if err := h.db.First(&camera, id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "Camera not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch camera"})
		return
	}

	// Get stream health status from MediaMTX
	isHealthy, err := h.mediamtxService.GetStreamHealth(camera.ID)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"camera_id":  camera.ID,
			"is_healthy": false,
			"error":      err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"camera_id":  camera.ID,
		"is_healthy": isHealthy,
	})
}

// GetWebRTCStream starts WebRTC stream for a camera
func (h *CameraHandler) GetWebRTCStream(c *gin.Context) {
	id := c.Param("id")

	var camera models.Camera
	if err := h.db.First(&camera, id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "Camera not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch camera"})
		return
	}

	// Start WebRTC stream with RTSP URL
	fmt.Printf("[WebRTC] Starting stream for camera %d (RTSP: %s)\n", camera.ID, camera.RTSPUrl)
	if err := h.webrtcService.StartStream(camera.ID, camera.RTSPUrl); err != nil {
		fmt.Printf("[WebRTC] Error starting stream for camera %d: %v\n", camera.ID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to start WebRTC stream: " + err.Error()})
		return
	}
	fmt.Printf("[WebRTC] Stream started successfully for camera %d\n", camera.ID)

	// Construct WebSocket URL
	// For development, always use localhost:8081 (backend port)
	// In production, use the request host
	var host string
	if os.Getenv("GIN_MODE") == "release" {
		// Production: use request host
		host = c.Request.Host
		if host == "" {
			host = "localhost:8081"
		}
		// If host doesn't have port, add default port based on scheme
		if !strings.Contains(host, ":") {
			if c.Request.TLS != nil || c.GetHeader("X-Forwarded-Proto") == "https" {
				host = host + ":443"
			} else {
				host = host + ":8081"
			}
		}
	} else {
		// Development: always use localhost:8081 (backend port from docker-compose)
		host = "localhost:8081"
	}

	// Determine scheme based on request
	scheme := "ws"
	if c.Request.TLS != nil {
		scheme = "wss"
	} else if c.GetHeader("X-Forwarded-Proto") == "https" {
		scheme = "wss"
	}

	// Construct WebSocket URL
	wsURL := fmt.Sprintf("%s://%s/api/v1/cameras/%d/webrtc/ws", scheme, host, camera.ID)
	fmt.Printf("[WebRTC] Generated WebSocket URL for camera %d: %s (request host: %s, mode: %s)\n", camera.ID, wsURL, c.Request.Host, os.Getenv("GIN_MODE"))

	c.JSON(http.StatusOK, gin.H{
		"camera_id":     camera.ID,
		"stream_type":   "webrtc",
		"websocket_url": wsURL,
	})
}

// HandleWebRTCWebSocket handles WebSocket connection for WebRTC signaling
func (h *CameraHandler) HandleWebRTCWebSocket(c *gin.Context) {
	id := c.Param("id")

	// Check authentication first (before upgrading)
	// Auth middleware should have validated token, but check user_id is set
	userID, exists := c.Get("user_id")
	if !exists {
		log.Printf("[WebRTC] WebSocket connection rejected: no authentication for camera %s\n", id)
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Authentication required"})
		return
	}
	log.Printf("[WebRTC] WebSocket connection from user %v for camera %s\n", userID, id)

	// Check camera exists before upgrading
	var camera models.Camera
	if err := h.db.First(&camera, id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			log.Printf("[WebRTC] Camera %s not found\n", id)
			c.JSON(http.StatusNotFound, gin.H{"error": "Camera not found"})
			return
		}
		log.Printf("[WebRTC] Error fetching camera %s: %v\n", id, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch camera"})
		return
	}

	log.Printf("[WebRTC] Upgrading to WebSocket for camera %d (RTSP: %s)\n", camera.ID, camera.RTSPUrl)

	// Upgrade to WebSocket - must be done before any response is written
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		// Can't use c.JSON after upgrade attempt fails, log error instead
		log.Printf("[WebRTC] WebSocket upgrade failed for camera %s: %v\n", id, err)
		return
	}

	log.Printf("[WebRTC] WebSocket upgraded successfully for camera %d\n", camera.ID)

	// Handle WebRTC signaling
	h.webrtcService.HandleWebSocket(conn, camera.ID)
}

// GetMJPEGStream streams MJPEG frames for a camera
// Simple HTTP streaming - no WebSocket, no file storage needed
func (h *CameraHandler) GetMJPEGStream(c *gin.Context) {
	id := c.Param("id")

	var camera models.Camera
	if err := h.db.First(&camera, id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "Camera not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch camera"})
		return
	}

	// Start MJPEG stream
	if err := h.mjpegService.StartStream(camera.ID, camera.RTSPUrl); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to start MJPEG stream: " + err.Error()})
		return
	}

	// Get stream reader
	reader, err := h.mjpegService.GetStreamReader(camera.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get MJPEG stream: " + err.Error()})
		return
	}
	defer reader.Close()

	// Set headers for MJPEG streaming
	// FFmpeg with -f mjpeg outputs multipart/x-mixed-replace automatically
	c.Header("Content-Type", "multipart/x-mixed-replace; boundary=ffmpeg")
	c.Header("Cache-Control", "no-cache, no-store, must-revalidate")
	c.Header("Pragma", "no-cache")
	c.Header("Expires", "0")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")

	fmt.Printf("[MJPEG] Starting stream for camera %d\n", camera.ID)

	// Stream MJPEG directly from FFmpeg
	// FFmpeg with -f mjpeg already outputs multipart/x-mixed-replace format
	// Just pipe it directly to HTTP response
	buffer := make([]byte, 8192)

	c.Stream(func(w io.Writer) bool {
		n, err := reader.Read(buffer)
		if n > 0 {
			if _, writeErr := w.Write(buffer[:n]); writeErr != nil {
				fmt.Printf("[MJPEG] Write error for camera %d: %v\n", camera.ID, writeErr)
				return false
			}
		}
		if err == io.EOF {
			fmt.Printf("[MJPEG] Stream ended for camera %d\n", camera.ID)
			return false
		}
		return err == nil
	})

	fmt.Printf("[MJPEG] Stream finished for camera %d\n", camera.ID)
}
