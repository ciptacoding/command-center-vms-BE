package services

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
)

type MJPEGService struct {
	activeStreams map[uint]*MJPEGStream
	mu            sync.RWMutex
}

type MJPEGStream struct {
	CameraID    uint
	RTSPURL     string
	FFmpegCmd   *exec.Cmd
	IsActive    bool
	mu          sync.RWMutex
}

func NewMJPEGService() *MJPEGService {
	return &MJPEGService{
		activeStreams: make(map[uint]*MJPEGStream),
	}
}

// StartStream starts RTSP to MJPEG conversion for a camera
// MJPEG streams JPEG frames continuously via HTTP multipart response
// No file storage needed - direct streaming to HTTP response
func (s *MJPEGService) StartStream(cameraID uint, rtspURL string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Check if stream already exists
	if stream, exists := s.activeStreams[cameraID]; exists && stream.IsActive {
		return nil
	}

	stream := &MJPEGStream{
		CameraID: cameraID,
		RTSPURL:  rtspURL,
		IsActive: false,
	}

	s.activeStreams[cameraID] = stream

	// Stream is started when HTTP request comes in
	// No need to start FFmpeg here - it will be started per connection

	return nil
}

// GetStreamReader returns a reader for MJPEG stream
// This will be used by HTTP handler to stream frames
func (s *MJPEGService) GetStreamReader(cameraID uint) (io.ReadCloser, error) {
	s.mu.RLock()
	stream, exists := s.activeStreams[cameraID]
	s.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("stream not found for camera %d", cameraID)
	}

	// Start FFmpeg to convert RTSP to MJPEG stream
	// Simple approach: use MJPEG format directly (multipart/x-mixed-replace)
	cmd := exec.Command("ffmpeg",
		"-rtsp_transport", "tcp",
		"-i", stream.RTSPURL,
		"-vf", "fps=15,scale=1280:720",
		"-q:v", "5",
		"-f", "mjpeg",
		"-",
		"-loglevel", "error",
	)
	
	// Capture stderr for debugging
	cmd.Stderr = os.Stderr

	// Get stdout pipe
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("error creating stdout pipe: %v", err)
	}

	// Start FFmpeg
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("error starting FFmpeg: %v", err)
	}

	stream.mu.Lock()
	stream.FFmpegCmd = cmd
	stream.IsActive = true
	stream.mu.Unlock()

	fmt.Printf("[MJPEG] Stream started for camera %d (RTSP: %s), PID: %d\n", cameraID, stream.RTSPURL, cmd.Process.Pid)
	
	// Check if process started successfully
	if cmd.Process == nil {
		return nil, fmt.Errorf("FFmpeg process not started")
	}

	// Return a reader that will close FFmpeg when done
	return &mjpegReader{
		reader: stdout,
		cmd:    cmd,
		stream: stream,
	}, nil
}

// mjpegReader wraps the FFmpeg stdout and ensures cleanup
type mjpegReader struct {
	reader io.ReadCloser
	cmd    *exec.Cmd
	stream *MJPEGStream
}

func (r *mjpegReader) Read(p []byte) (n int, err error) {
	return r.reader.Read(p)
}

func (r *mjpegReader) Close() error {
	if r.cmd != nil && r.cmd.Process != nil {
		fmt.Printf("[MJPEG] Stopping FFmpeg for camera %d (PID: %d)\n", r.stream.CameraID, r.cmd.Process.Pid)
		r.cmd.Process.Kill()
		r.cmd.Wait()
	}
	r.stream.mu.Lock()
	r.stream.IsActive = false
	r.stream.mu.Unlock()
	return r.reader.Close()
}

// StopStream stops MJPEG stream for a camera
func (s *MJPEGService) StopStream(cameraID uint) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	stream, exists := s.activeStreams[cameraID]
	if !exists {
		return fmt.Errorf("stream not found for camera %d", cameraID)
	}

	// Stop FFmpeg if running
	stream.mu.Lock()
	if stream.FFmpegCmd != nil && stream.FFmpegCmd.Process != nil {
		stream.FFmpegCmd.Process.Kill()
		stream.FFmpegCmd.Wait()
	}
	stream.mu.Unlock()

	delete(s.activeStreams, cameraID)
	return nil
}

// GetStreamStatus returns the status of a stream
func (s *MJPEGService) GetStreamStatus(cameraID uint) (bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	stream, exists := s.activeStreams[cameraID]
	if !exists {
		return false, fmt.Errorf("stream not found for camera %d", cameraID)
	}

	stream.mu.RLock()
	defer stream.mu.RUnlock()
	return stream.IsActive, nil
}

