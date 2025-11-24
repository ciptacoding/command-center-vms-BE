package services

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"command-center-vms-cctv/be/config"
)

type RTSPService struct {
	config        config.RTSPConfig
	activeStreams map[uint]*StreamInfo // camera_id -> stream info
	mu            sync.RWMutex
	stopMonitor   chan struct{}
}

type StreamInfo struct {
	HLSURL      string
	FFmpegCmd   *exec.Cmd
	RTSPURL     string
	OutputPath  string
	CameraID    uint
	LastUpdate  time.Time
	RestartCount int
	IsHealthy   bool
}

func NewRTSPService(cfg config.RTSPConfig) *RTSPService {
	// Create output directory if it doesn't exist
	if err := os.MkdirAll(cfg.OutputPath, 0755); err != nil {
		fmt.Printf("Warning: Failed to create HLS output directory: %v\n", err)
	}

	service := &RTSPService{
		config:        cfg,
		activeStreams: make(map[uint]*StreamInfo),
		stopMonitor:   make(chan struct{}),
	}

	// Start monitoring goroutine
	go service.monitorStreams()

	return service
}

// monitorStreams periodically checks stream health and auto-restarts if needed
func (s *RTSPService) monitorStreams() {
	ticker := time.NewTicker(10 * time.Second) // Check every 10 seconds
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			s.checkStreamHealth()
		case <-s.stopMonitor:
			return
		}
	}
}

// checkStreamHealth checks all active streams and restarts unhealthy ones
func (s *RTSPService) checkStreamHealth() {
	s.mu.Lock()
	defer s.mu.Unlock()

	for cameraID, streamInfo := range s.activeStreams {
		// Check if FFmpeg process is still running
		if streamInfo.FFmpegCmd != nil && streamInfo.FFmpegCmd.Process != nil {
			// Check if process is still alive
			if err := streamInfo.FFmpegCmd.Process.Signal(os.Signal(nil)); err != nil {
				// Process is dead, restart stream
				fmt.Printf("FFmpeg process for camera %d is dead, restarting...\n", cameraID)
				s.restartStreamUnsafe(cameraID, streamInfo)
				continue
			}
		} else {
			// Process doesn't exist, restart stream
			fmt.Printf("FFmpeg process for camera %d doesn't exist, restarting...\n", cameraID)
			s.restartStreamUnsafe(cameraID, streamInfo)
			continue
		}

		// Check if playlist file exists and is being updated
		playlistPath := streamInfo.OutputPath
		if fileInfo, err := os.Stat(playlistPath); err == nil {
			// Check if file was updated in the last 30 seconds (should update every 2-6 seconds for HLS)
			timeSinceUpdate := time.Since(fileInfo.ModTime())
			if timeSinceUpdate > 30*time.Second {
				fmt.Printf("Playlist file for camera %d hasn't been updated in %v, restarting stream...\n", cameraID, timeSinceUpdate)
				s.restartStreamUnsafe(cameraID, streamInfo)
				continue
			}
			streamInfo.LastUpdate = fileInfo.ModTime()
			streamInfo.IsHealthy = true
		} else {
			// Playlist file doesn't exist, restart stream
			fmt.Printf("Playlist file for camera %d doesn't exist, restarting stream...\n", cameraID)
			s.restartStreamUnsafe(cameraID, streamInfo)
			continue
		}
	}
}

// restartStreamUnsafe restarts a stream (must be called with lock held)
func (s *RTSPService) restartStreamUnsafe(cameraID uint, streamInfo *StreamInfo) {
	// Stop existing process
	if streamInfo.FFmpegCmd != nil && streamInfo.FFmpegCmd.Process != nil {
		streamInfo.FFmpegCmd.Process.Kill()
	}

	// Limit restart attempts (max 5 times)
	if streamInfo.RestartCount >= 5 {
		fmt.Printf("Camera %d has exceeded max restart attempts, marking as unhealthy\n", cameraID)
		streamInfo.IsHealthy = false
		return
	}

	streamInfo.RestartCount++
	streamInfo.IsHealthy = false

	// Restart stream in goroutine
	go s.convertRTSPToHLS(streamInfo.RTSPURL, streamInfo.OutputPath, cameraID, streamInfo)
}

func (s *RTSPService) StartStream(cameraID uint, rtspURL string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Check if stream already exists
	if streamInfo, exists := s.activeStreams[cameraID]; exists {
		return streamInfo.HLSURL, nil
	}

	// Generate HLS output path for this camera
	hlsPath := filepath.Join(s.config.OutputPath, fmt.Sprintf("camera_%d", cameraID))
	if err := os.MkdirAll(hlsPath, 0755); err != nil {
		return "", fmt.Errorf("failed to create HLS directory: %w", err)
	}

	// HLS playlist file
	playlistFile := filepath.Join(hlsPath, "playlist.m3u8")
	
	// HLS URL for frontend
	hlsURL := fmt.Sprintf("%s/camera_%d/playlist.m3u8", s.config.StreamPath, cameraID)

	// Start RTSP to HLS conversion using FFmpeg
	// FFmpeg is the standard tool for RTSP to HLS conversion
	streamInfo := &StreamInfo{
		HLSURL:      hlsURL,
		RTSPURL:     rtspURL,
		OutputPath:  playlistFile,
		CameraID:    cameraID,
		LastUpdate:  time.Now(),
		RestartCount: 0,
		IsHealthy:   false,
	}

	// Start conversion in goroutine
	go s.convertRTSPToHLS(rtspURL, playlistFile, cameraID, streamInfo)

	// Store the stream
	s.activeStreams[cameraID] = streamInfo

	return hlsURL, nil
}

func (s *RTSPService) convertRTSPToHLS(rtspURL, outputPath string, cameraID uint, streamInfo *StreamInfo) {
	// Check if ffmpeg is available
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		fmt.Printf("Error: ffmpeg not found. RTSP to HLS conversion requires ffmpeg to be installed.\n")
		fmt.Printf("Install ffmpeg: https://ffmpeg.org/download.html\n")
		fmt.Printf("For macOS: brew install ffmpeg\n")
		fmt.Printf("For Ubuntu/Debian: sudo apt-get install ffmpeg\n")
		
		// Remove from active streams on error
		s.mu.Lock()
		delete(s.activeStreams, cameraID)
		s.mu.Unlock()
		return
	}

	// FFmpeg command with optimized settings for RTSP to HLS conversion
	// Similar to RTSPtoWeb approach but using FFmpeg for HLS output
	cmd := exec.Command("ffmpeg",
		"-rtsp_transport", "tcp",        // Use TCP for better reliability (RTSPtoWeb approach)
		"-i", rtspURL,
		"-c:v", "libx264",               // Video codec
		"-preset", "ultrafast",          // Fast encoding for low latency
		"-tune", "zerolatency",          // Zero latency tuning
		"-g", "50",                       // GOP size
		"-c:a", "aac",                   // Audio codec
		"-b:a", "128k",                  // Audio bitrate
		"-f", "hls",                     // Output format
		"-hls_time", "2",                // Segment duration in seconds
		"-hls_list_size", "3",           // Number of segments in playlist
		"-hls_flags", "delete_segments", // Delete old segments
		"-hls_segment_type", "mpegts",   // Segment type
		"-hls_segment_filename", filepath.Join(filepath.Dir(outputPath), "segment_%03d.ts"),
		"-start_number", "0",
		"-hls_allow_cache", "0",         // Disable cache for live streaming
		"-hls_start_number_source", "epoch", // Use epoch for segment numbering
		outputPath,
	)

	// Set output to capture errors
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	streamInfo.FFmpegCmd = cmd

	fmt.Printf("Starting RTSP to HLS conversion for camera %d: %s -> %s\n", cameraID, rtspURL, outputPath)
	
	// Start the command
	if err := cmd.Start(); err != nil {
		fmt.Printf("Error starting FFmpeg for camera %d: %v\n", cameraID, err)
		s.mu.Lock()
		streamInfo.IsHealthy = false
		s.mu.Unlock()
		return
	}

	// Mark as healthy initially
	s.mu.Lock()
	streamInfo.IsHealthy = true
	streamInfo.RestartCount = 0 // Reset restart count on successful start
	s.mu.Unlock()

	// Wait for command to finish (or error)
	if err := cmd.Wait(); err != nil {
		fmt.Printf("FFmpeg process for camera %d exited with error: %v\n", cameraID, err)
		s.mu.Lock()
		streamInfo.IsHealthy = false
		// Don't delete here, let monitor restart it
		s.mu.Unlock()
	}
}

func (s *RTSPService) StopStream(cameraID uint) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	streamInfo, exists := s.activeStreams[cameraID]
	if !exists {
		return fmt.Errorf("stream not found for camera %d", cameraID)
	}

	// Stop FFmpeg process if running
	if streamInfo.FFmpegCmd != nil && streamInfo.FFmpegCmd.Process != nil {
		if err := streamInfo.FFmpegCmd.Process.Kill(); err != nil {
			fmt.Printf("Error stopping FFmpeg process for camera %d: %v\n", cameraID, err)
		}
	}

	delete(s.activeStreams, cameraID)
	return nil
}

func (s *RTSPService) GetStreamURL(cameraID uint) (string, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	streamInfo, exists := s.activeStreams[cameraID]
	if !exists {
		return "", false
	}
	return streamInfo.HLSURL, true
}

// GetStreamHealth returns the health status of a stream
func (s *RTSPService) GetStreamHealth(cameraID uint) (bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	streamInfo, exists := s.activeStreams[cameraID]
	if !exists {
		return false, fmt.Errorf("stream not found for camera %d", cameraID)
	}
	
	return streamInfo.IsHealthy, nil
}

// GetAllStreamHealth returns health status of all streams
func (s *RTSPService) GetAllStreamHealth() map[uint]bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	health := make(map[uint]bool)
	for cameraID, streamInfo := range s.activeStreams {
		health[cameraID] = streamInfo.IsHealthy
	}
	
	return health
}
