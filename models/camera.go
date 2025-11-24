package models

import (
	"time"

	"gorm.io/gorm"
)

type Camera struct {
	ID              uint           `json:"id" gorm:"primaryKey"`
	Name            string         `json:"name" gorm:"not null"`
	Latitude        float64        `json:"latitude" gorm:"not null"`
	Longitude       float64        `json:"longitude" gorm:"not null"`
	RTSPUrl         string         `json:"rtsp_url" gorm:"not null"`
	Status          string         `json:"status" gorm:"default:offline"` // online, offline
	Area            string         `json:"area" gorm:"not null"`
	Building        string         `json:"building" gorm:"not null"`
	LastMotionDetected *time.Time  `json:"last_motion_detected,omitempty"`
	CreatedAt       time.Time      `json:"created_at"`
	UpdatedAt       time.Time      `json:"updated_at"`
	DeletedAt       gorm.DeletedAt `json:"-" gorm:"index"`
}

