package database

import (
	"fmt"
	"log"

	"command-center-vms-cctv/be/config"
	"command-center-vms-cctv/be/models"
	"command-center-vms-cctv/be/utils"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func Initialize(cfg config.DatabaseConfig) (*gorm.DB, error) {
	dsn := fmt.Sprintf(
		"host=%s user=%s password=%s dbname=%s port=%s sslmode=%s",
		cfg.Host, cfg.User, cfg.Password, cfg.DBName, cfg.Port, cfg.SSLMode,
	)

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Info),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	// Auto migrate
	if err := db.AutoMigrate(
		&models.User{},
		&models.Camera{},
	); err != nil {
		return nil, fmt.Errorf("failed to migrate database: %w", err)
	}

	// Create default admin user if not exists
	if err := createDefaultAdmin(db); err != nil {
		log.Printf("Warning: Failed to create default admin: %v", err)
	}

	log.Println("Database initialized successfully")
	return db, nil
}

func createDefaultAdmin(db *gorm.DB) error {
	var count int64
	db.Model(&models.User{}).Count(&count)

	if count > 0 {
		return nil // Admin already exists
	}

	// Generate password hash for "demo123"
	hashedPassword, err := utils.HashPassword("demo123")
	if err != nil {
		return fmt.Errorf("failed to hash password: %w", err)
	}

	// Create default admin user
	admin := &models.User{
		Email:    "admin@vms.demo",
		Name:     "Admin User",
		Password: hashedPassword,
		Role:     "admin",
	}

	if err := db.Create(admin).Error; err != nil {
		return err
	}

	log.Println("Default admin user created: admin@vms.demo / demo123")
	return nil
}

