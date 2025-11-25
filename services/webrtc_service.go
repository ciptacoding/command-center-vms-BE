package services

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/pion/webrtc/v3"
	"github.com/pion/webrtc/v3/pkg/media"
)

type WebRTCService struct {
	activeStreams map[uint]*WebRTCStream
	mu            sync.RWMutex
	api           *webrtc.API
}

type WebRTCStream struct {
	CameraID         uint
	RTSPURL          string
	PeerConnections  map[string]*webrtc.PeerConnection
	VideoTrack       *webrtc.TrackLocalStaticSample
	IsActive         bool
	FFmpegCmd        *exec.Cmd
	FFmpegStdin      io.WriteCloser
	mu               sync.RWMutex
}

type SignalingMessage struct {
	Type      string          `json:"type"`      // "offer", "answer", "ice-candidate"
	CameraID  uint            `json:"camera_id"`
	SDP       string          `json:"sdp,omitempty"`
	Candidate json.RawMessage `json:"candidate,omitempty"`
}

func NewWebRTCService() *WebRTCService {
	// Configure WebRTC API with VP8 codec for video
	mediaEngine := &webrtc.MediaEngine{}
	
	// Register VP8 codec for video
	if err := mediaEngine.RegisterCodec(webrtc.RTPCodecParameters{
		RTPCodecCapability: webrtc.RTPCodecCapability{
			MimeType:     webrtc.MimeTypeVP8,
			ClockRate:    90000,
			Channels:     0,
			SDPFmtpLine:  "",
			RTCPFeedback: nil,
		},
		PayloadType: 96,
	}, webrtc.RTPCodecTypeVideo); err != nil {
		panic(err)
	}

	// Register Opus codec for audio (optional)
	if err := mediaEngine.RegisterCodec(webrtc.RTPCodecParameters{
		RTPCodecCapability: webrtc.RTPCodecCapability{
			MimeType:     webrtc.MimeTypeOpus,
			ClockRate:    48000,
			Channels:     2,
			SDPFmtpLine:  "",
			RTCPFeedback: nil,
		},
		PayloadType: 111,
	}, webrtc.RTPCodecTypeAudio); err != nil {
		panic(err)
	}

	api := webrtc.NewAPI(webrtc.WithMediaEngine(mediaEngine))

	return &WebRTCService{
		activeStreams: make(map[uint]*WebRTCStream),
		api:           api,
	}
}

// StartStream starts RTSP to WebRTC conversion for a camera
func (s *WebRTCService) StartStream(cameraID uint, rtspURL string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Check if stream already exists
	if stream, exists := s.activeStreams[cameraID]; exists && stream.IsActive {
		return nil
	}

	stream := &WebRTCStream{
		CameraID:        cameraID,
		RTSPURL:         rtspURL,
		PeerConnections: make(map[string]*webrtc.PeerConnection),
		IsActive:        false,
	}

	s.activeStreams[cameraID] = stream

	// Start RTSP to WebRTC conversion
	go s.convertRTSPToWebRTC(stream)

	return nil
}

// convertRTSPToWebRTC converts RTSP stream to WebRTC using FFmpeg
// FFmpeg decodes RTSP, encodes to VP8, and outputs to stdout (in-memory, no disk storage)
// We read VP8 frames from stdout and send directly to WebRTC track
func (s *WebRTCService) convertRTSPToWebRTC(stream *WebRTCStream) {
	// Create video track
	videoTrack, err := webrtc.NewTrackLocalStaticSample(
		webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeVP8},
		"video",
		fmt.Sprintf("camera_%d", stream.CameraID),
	)
	if err != nil {
		fmt.Printf("Error creating video track for camera %d: %v\n", stream.CameraID, err)
		return
	}

	stream.mu.Lock()
	stream.VideoTrack = videoTrack
	stream.mu.Unlock()

	// FFmpeg command to decode RTSP and encode to VP8
	// Output to stdout (in-memory, no file storage)
	// Using VP8 codec for WebRTC compatibility
	// Note: If libvpx is not available, FFmpeg will error and we'll handle it
	cmd := exec.Command("ffmpeg",
		"-rtsp_transport", "tcp",        // Use TCP for better reliability
		"-i", stream.RTSPURL,            // RTSP input
		"-c:v", "libvpx",                // VP8 video codec (WebRTC compatible)
		"-deadline", "realtime",         // Real-time encoding
		"-cpu-used", "8",                // Fast encoding (0-8, 8 is fastest)
		"-b:v", "1M",                    // Video bitrate
		"-maxrate", "1M",                // Max bitrate
		"-bufsize", "2M",                // Buffer size
		"-g", "30",                       // GOP size (keyframe interval)
		"-keyint_min", "30",             // Minimum keyframe interval
		"-f", "ivf",                     // IVF format (VP8 container, easy to parse)
		"-",                             // Output to stdout (in-memory)
		"-loglevel", "warning",          // Show warnings and errors for debugging
	)
	
	// Capture stderr for error messages
	cmd.Stderr = os.Stderr

	// Get stdout pipe
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		fmt.Printf("Error creating stdout pipe for camera %d: %v\n", stream.CameraID, err)
		return
	}

	// Get stdin pipe (for potential control)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		fmt.Printf("Error creating stdin pipe for camera %d: %v\n", stream.CameraID, err)
		return
	}

	stream.mu.Lock()
	stream.FFmpegCmd = cmd
	stream.FFmpegStdin = stdin
	stream.mu.Unlock()

	// Start FFmpeg
	if err := cmd.Start(); err != nil {
		fmt.Printf("[WebRTC] Error starting FFmpeg for camera %d: %v\n", stream.CameraID, err)
		stream.mu.Lock()
		stream.IsActive = false
		stream.mu.Unlock()
		return
	}

	fmt.Printf("[WebRTC] Stream started for camera %d (RTSP: %s)\n", stream.CameraID, stream.RTSPURL)
	fmt.Printf("[WebRTC] FFmpeg PID: %d\n", cmd.Process.Pid)

	stream.mu.Lock()
	stream.IsActive = true
	stream.mu.Unlock()

	// Read VP8 frames from FFmpeg stdout and send to WebRTC track
	go s.readAndSendVP8Frames(stdout, videoTrack, stream.CameraID)

	// Wait for FFmpeg to finish (or error)
	go func() {
		if err := cmd.Wait(); err != nil {
			fmt.Printf("FFmpeg process ended for camera %d: %v\n", stream.CameraID, err)
		}
		
		// Mark stream as inactive
		stream.mu.Lock()
		stream.IsActive = false
		stream.mu.Unlock()
	}()
}

// readAndSendVP8Frames reads VP8 frames from FFmpeg stdout and sends to WebRTC track
// IVF format structure:
// - 32 bytes header
// - Frame: 4 bytes size + frame data
func (s *WebRTCService) readAndSendVP8Frames(stdout io.Reader, track *webrtc.TrackLocalStaticSample, cameraID uint) {
	reader := bufio.NewReader(stdout)
	
	// Read IVF header (32 bytes)
	header := make([]byte, 32)
	if _, err := io.ReadFull(reader, header); err != nil {
		fmt.Printf("Error reading IVF header for camera %d: %v\n", cameraID, err)
		return
	}

	// Verify IVF header signature
	if string(header[0:4]) != "DKIF" {
		fmt.Printf("Invalid IVF header for camera %d\n", cameraID)
		return
	}

	fmt.Printf("[WebRTC] Reading VP8 frames for camera %d...\n", cameraID)
	
	// Frame timing for 30 FPS (33.33ms per frame)
	frameDuration := time.Duration(33_333_333) // 33.33ms in nanoseconds
	lastFrameTime := time.Now()

	// Read frames continuously
	for {
		// Read frame size (4 bytes, little-endian)
		sizeBytes := make([]byte, 4)
		if _, err := io.ReadFull(reader, sizeBytes); err != nil {
			if err == io.EOF {
				fmt.Printf("FFmpeg stdout closed for camera %d\n", cameraID)
				break
			}
			fmt.Printf("Error reading frame size for camera %d: %v\n", cameraID, err)
			break
		}

		// Parse frame size (little-endian uint32)
		frameSize := uint32(sizeBytes[0]) | uint32(sizeBytes[1])<<8 | uint32(sizeBytes[2])<<16 | uint32(sizeBytes[3])<<24
		
		if frameSize == 0 {
			fmt.Printf("Zero frame size for camera %d, skipping\n", cameraID)
			continue
		}

		// Read frame data
		frameData := make([]byte, frameSize)
		if _, err := io.ReadFull(reader, frameData); err != nil {
			if err == io.EOF {
				fmt.Printf("FFmpeg stdout closed for camera %d\n", cameraID)
				break
			}
			fmt.Printf("Error reading frame data for camera %d: %v\n", cameraID, err)
			break
		}

		// Calculate timing for this frame
		now := time.Now()
		elapsed := now.Sub(lastFrameTime)
		
		// If we're behind, catch up; if ahead, wait
		if elapsed < frameDuration {
			time.Sleep(frameDuration - elapsed)
		}
		
		// Send frame to WebRTC track
		if err := track.WriteSample(media.Sample{
			Data:     frameData,
			Duration: frameDuration,
		}); err != nil {
			fmt.Printf("Error writing sample to track for camera %d: %v\n", cameraID, err)
			// Continue reading even if write fails (might be no peer connections yet)
		}

		lastFrameTime = time.Now()
	}

	fmt.Printf("Stopped reading VP8 frames for camera %d\n", cameraID)
}

// Note: readRTPPackets function removed - not needed in simplified implementation
// Full RTSP to WebRTC conversion requires complex RTP packet parsing

// HandleWebSocket handles WebSocket connection for WebRTC signaling
func (s *WebRTCService) HandleWebSocket(conn *websocket.Conn, cameraID uint) {
	defer conn.Close()

	stream, exists := s.activeStreams[cameraID]
	if !exists {
		conn.WriteJSON(map[string]string{"error": "Stream not found. Please start stream first."})
		return
	}

	// Wait for stream to be ready
	for i := 0; i < 10; i++ {
		stream.mu.RLock()
		isActive := stream.IsActive
		videoTrack := stream.VideoTrack
		stream.mu.RUnlock()

		if isActive && videoTrack != nil {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	// Create peer connection
	peerConnection, err := s.api.NewPeerConnection(webrtc.Configuration{
		ICEServers: []webrtc.ICEServer{
			{URLs: []string{"stun:stun.l.google.com:19302"}},
		},
	})
	if err != nil {
		conn.WriteJSON(map[string]string{"error": fmt.Sprintf("Failed to create peer connection: %v", err)})
		return
	}
	defer peerConnection.Close()

	// Store peer connection
	connID := fmt.Sprintf("%p", conn)
	stream.mu.Lock()
	if stream.PeerConnections == nil {
		stream.PeerConnections = make(map[string]*webrtc.PeerConnection)
	}
	stream.PeerConnections[connID] = peerConnection
	stream.mu.Unlock()

	// Add video track
	stream.mu.RLock()
	videoTrack := stream.VideoTrack
	stream.mu.RUnlock()

	if videoTrack != nil {
		if _, err := peerConnection.AddTrack(videoTrack); err != nil {
			conn.WriteJSON(map[string]string{"error": fmt.Sprintf("Failed to add track: %v", err)})
			return
		}
	} else {
		conn.WriteJSON(map[string]string{"error": "Video track not available"})
		return
	}

	// Handle ICE candidates
	peerConnection.OnICECandidate(func(candidate *webrtc.ICECandidate) {
		if candidate != nil {
			candidateJSON, _ := json.Marshal(candidate.ToJSON())
			conn.WriteJSON(map[string]interface{}{
				"type":      "ice-candidate",
				"candidate": json.RawMessage(candidateJSON),
			})
		}
	})

	// Handle connection state
	peerConnection.OnConnectionStateChange(func(state webrtc.PeerConnectionState) {
		fmt.Printf("Camera %d WebRTC connection state: %s\n", cameraID, state.String())
		if state == webrtc.PeerConnectionStateClosed || state == webrtc.PeerConnectionStateFailed {
			// Remove peer connection
			stream.mu.Lock()
			delete(stream.PeerConnections, connID)
			stream.mu.Unlock()
			conn.Close()
		}
	})

	// Read messages from client
	for {
		var msg SignalingMessage
		if err := conn.ReadJSON(&msg); err != nil {
			fmt.Printf("Error reading WebSocket message: %v\n", err)
			break
		}

		switch msg.Type {
		case "offer":
			// Set remote description
			offer := webrtc.SessionDescription{
				Type: webrtc.SDPTypeOffer,
				SDP:  msg.SDP,
			}
			if err := peerConnection.SetRemoteDescription(offer); err != nil {
				conn.WriteJSON(map[string]string{"error": fmt.Sprintf("Failed to set remote description: %v", err)})
				continue
			}

			// Create answer
			answer, err := peerConnection.CreateAnswer(nil)
			if err != nil {
				conn.WriteJSON(map[string]string{"error": fmt.Sprintf("Failed to create answer: %v", err)})
				continue
			}

			// Set local description
			if err := peerConnection.SetLocalDescription(answer); err != nil {
				conn.WriteJSON(map[string]string{"error": fmt.Sprintf("Failed to set local description: %v", err)})
				continue
			}

			// Send answer
			conn.WriteJSON(map[string]interface{}{
				"type": "answer",
				"sdp":  answer.SDP,
			})

		case "ice-candidate":
			// Add ICE candidate
			var candidate webrtc.ICECandidateInit
			if err := json.Unmarshal(msg.Candidate, &candidate); err != nil {
				fmt.Printf("Error parsing ICE candidate: %v\n", err)
				continue
			}
			if err := peerConnection.AddICECandidate(candidate); err != nil {
				fmt.Printf("Error adding ICE candidate: %v\n", err)
			}
		}
	}
}

// StopStream stops WebRTC stream for a camera
func (s *WebRTCService) StopStream(cameraID uint) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	stream, exists := s.activeStreams[cameraID]
	if !exists {
		return fmt.Errorf("stream not found for camera %d", cameraID)
	}

	// Stop FFmpeg process
	stream.mu.Lock()
	if stream.FFmpegCmd != nil && stream.FFmpegCmd.Process != nil {
		fmt.Printf("Stopping FFmpeg for camera %d (PID: %d)\n", cameraID, stream.FFmpegCmd.Process.Pid)
		stream.FFmpegCmd.Process.Kill()
		stream.FFmpegCmd.Wait()
	}
	if stream.FFmpegStdin != nil {
		stream.FFmpegStdin.Close()
	}
	stream.mu.Unlock()

	// Close all peer connections
	stream.mu.Lock()
	for _, pc := range stream.PeerConnections {
		pc.Close()
	}
	stream.mu.Unlock()

	delete(s.activeStreams, cameraID)
	return nil
}

// GetStreamStatus returns the status of a stream
func (s *WebRTCService) GetStreamStatus(cameraID uint) (bool, error) {
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

