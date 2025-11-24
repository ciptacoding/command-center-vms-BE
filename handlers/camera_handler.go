package handlers

import (
	"net/http"

	"command-center-vms-cctv/be/models"
	"command-center-vms-cctv/be/services"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type CameraHandler struct {
	db          *gorm.DB
	rtspService *services.RTSPService
}

func NewCameraHandler(db *gorm.DB, rtspService *services.RTSPService) *CameraHandler {
	return &CameraHandler{
		db:          db,
		rtspService: rtspService,
	}
}

type CreateCameraRequest struct {
	Name     string  `json:"name" binding:"required"`
	Latitude float64 `json:"latitude" binding:"required"`
	Longitude float64 `json:"longitude" binding:"required"`
	RTSPUrl  string  `json:"rtsp_url" binding:"required"`
	Area     string  `json:"area" binding:"required"`
	Building string  `json:"building" binding:"required"`
	Status   string  `json:"status"`
}

type UpdateCameraRequest struct {
	Name     *string  `json:"name"`
	Latitude *float64 `json:"latitude"`
	Longitude *float64 `json:"longitude"`
	RTSPUrl  *string  `json:"rtsp_url"`
	Area     *string  `json:"area"`
	Building *string  `json:"building"`
	Status   *string  `json:"status"`
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
		Name:     req.Name,
		Latitude: req.Latitude,
		Longitude: req.Longitude,
		RTSPUrl:  req.RTSPUrl,
		Status:   status,
		Area:     req.Area,
		Building: req.Building,
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

	// Start RTSP stream and get HLS URL
	hlsURL, err := h.rtspService.StartStream(camera.ID, camera.RTSPUrl)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to start stream: " + err.Error()})
		return
	}

	// Get stream health status
	isHealthy, _ := h.rtspService.GetStreamHealth(camera.ID)

	c.JSON(http.StatusOK, gin.H{
		"hls_url": hlsURL,
		"camera_id": camera.ID,
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

	// Get stream health status
	isHealthy, err := h.rtspService.GetStreamHealth(camera.ID)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"camera_id": camera.ID,
			"is_healthy": false,
			"error": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"camera_id": camera.ID,
		"is_healthy": isHealthy,
	})
}

