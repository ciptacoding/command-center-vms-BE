package services

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"command-center-vms-cctv/be/config"
)

type MediaMTXService struct {
	config      config.MediaMTXConfig
	httpClient  *http.Client
	activePaths map[uint]string // camera_id -> path_name
	mu          sync.RWMutex
}

func NewMediaMTXService(cfg config.MediaMTXConfig) *MediaMTXService {
	return &MediaMTXService{
		config:      cfg,
		httpClient:  &http.Client{Timeout: 10 * time.Second},
		activePaths: make(map[uint]string),
	}
}

// GetPathName returns the MediaMTX path name for a camera
func (s *MediaMTXService) GetPathName(cameraID uint) string {
	return fmt.Sprintf("cam%d", cameraID)
}

// StartStream configures a MediaMTX path for a camera and returns the HLS URL
// MediaMTX will pull RTSP stream from the camera and serve it as HLS
func (s *MediaMTXService) StartStream(cameraID uint, rtspURL string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Check if path already exists
	if pathName, exists := s.activePaths[cameraID]; exists {
		// Use PublicHost for HLS URL so browser can access it
		hlsURL := fmt.Sprintf("http://%s:%s/%s/index.m3u8", s.config.PublicHost, s.config.HTTPPort, pathName)
		return hlsURL, nil
	}

	pathName := s.GetPathName(cameraID)

	// Configure path in MediaMTX via API
	// MediaMTX uses config patch API to add paths dynamically
	pathConfig := map[string]interface{}{
		"source":                     rtspURL,
		"sourceOnDemand":             true,
		"sourceOnDemandStartTimeout": "10s",
		"sourceOnDemandCloseAfter":   "10s",
		"sourceProtocol":             "tcp",
		"sourceAnyPortEnable":        false,
	}

	// Use config patch API to add path
	// Format: {"paths": {"pathName": {...config...}}}
	patchConfig := map[string]interface{}{
		"paths": map[string]interface{}{
			pathName: pathConfig,
		},
	}

	configJSON, err := json.Marshal(patchConfig)
	if err != nil {
		return "", fmt.Errorf("failed to marshal path config: %w", err)
	}

	// MediaMTX v2 API: POST /v2/config/patch
	configURL := fmt.Sprintf("http://%s:%s/v2/config/patch", s.config.Host, s.config.APIPort)

	req, err := http.NewRequest("POST", configURL, bytes.NewBuffer(configJSON))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to configure MediaMTX path: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("MediaMTX API error (status %d): %s", resp.StatusCode, string(body))
	}

	// Store active path
	s.activePaths[cameraID] = pathName

	// Construct HLS URL using PublicHost so browser can access it
	hlsURL := fmt.Sprintf("http://%s:%s/%s/index.m3u8", s.config.PublicHost, s.config.HTTPPort, pathName)

	fmt.Printf("[MediaMTX] Path configured for camera %d: %s (RTSP: %s) -> HLS: %s\n", cameraID, pathName, rtspURL, hlsURL)

	return hlsURL, nil
}

// StopStream removes a MediaMTX path for a camera
func (s *MediaMTXService) StopStream(cameraID uint) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	pathName, exists := s.activePaths[cameraID]
	if !exists {
		return fmt.Errorf("stream not found for camera %d", cameraID)
	}

	// Remove path from MediaMTX using config patch API
	// Set path to null to remove it
	patchConfig := map[string]interface{}{
		"paths": map[string]interface{}{
			pathName: nil,
		},
	}

	configJSON, err := json.Marshal(patchConfig)
	if err != nil {
		return fmt.Errorf("failed to marshal patch config: %w", err)
	}

	configURL := fmt.Sprintf("http://%s:%s/v2/config/patch", s.config.Host, s.config.APIPort)

	req, err := http.NewRequest("POST", configURL, bytes.NewBuffer(configJSON))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to remove MediaMTX path: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("MediaMTX API error (status %d): %s", resp.StatusCode, string(body))
	}

	delete(s.activePaths, cameraID)
	fmt.Printf("[MediaMTX] Path removed for camera %d: %s\n", cameraID, pathName)

	return nil
}

// GetStreamURL returns the HLS URL for a camera if the stream is active
func (s *MediaMTXService) GetStreamURL(cameraID uint) (string, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	pathName, exists := s.activePaths[cameraID]
	if !exists {
		return "", false
	}

	// Use PublicHost for HLS URL so browser can access it
	hlsURL := fmt.Sprintf("http://%s:%s/%s/index.m3u8", s.config.PublicHost, s.config.HTTPPort, pathName)
	return hlsURL, true
}

// GetStreamHealth checks if a MediaMTX path is active and healthy
func (s *MediaMTXService) GetStreamHealth(cameraID uint) (bool, error) {
	s.mu.RLock()
	pathName, exists := s.activePaths[cameraID]
	s.mu.RUnlock()

	if !exists {
		return false, fmt.Errorf("stream not found for camera %d", cameraID)
	}

	// Check path status via MediaMTX API
	statusURL := fmt.Sprintf("http://%s:%s/v2/paths/list", s.config.Host, s.config.APIPort)

	resp, err := s.httpClient.Get(statusURL)
	if err != nil {
		return false, fmt.Errorf("failed to check MediaMTX path status: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("MediaMTX API error (status %d)", resp.StatusCode)
	}

	var pathsResponse map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&pathsResponse); err != nil {
		return false, fmt.Errorf("failed to decode MediaMTX response: %w", err)
	}

	// Check if path exists in response
	if paths, ok := pathsResponse["items"].(map[string]interface{}); ok {
		if _, exists := paths[pathName]; exists {
			return true, nil
		}
	}

	return false, nil
}

// GetAllStreamHealth returns health status of all active streams
func (s *MediaMTXService) GetAllStreamHealth() map[uint]bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	health := make(map[uint]bool)

	// Get all paths from MediaMTX
	statusURL := fmt.Sprintf("http://%s:%s/v2/paths/list", s.config.Host, s.config.APIPort)
	resp, err := s.httpClient.Get(statusURL)
	if err != nil {
		// If API call fails, mark all as unhealthy
		for cameraID := range s.activePaths {
			health[cameraID] = false
		}
		return health
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		for cameraID := range s.activePaths {
			health[cameraID] = false
		}
		return health
	}

	var pathsResponse map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&pathsResponse); err != nil {
		for cameraID := range s.activePaths {
			health[cameraID] = false
		}
		return health
	}

	// Build map of active paths from MediaMTX
	activePaths := make(map[string]bool)
	if paths, ok := pathsResponse["items"].(map[string]interface{}); ok {
		for pathName := range paths {
			activePaths[pathName] = true
		}
	}

	// Check each camera's path
	for cameraID, pathName := range s.activePaths {
		health[cameraID] = activePaths[pathName]
	}

	return health
}
