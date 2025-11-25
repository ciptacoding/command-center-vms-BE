package main

import (
	"fmt"
	"log"
	"os"

	"command-center-vms-cctv/be/config"
	"command-center-vms-cctv/be/database"
	"command-center-vms-cctv/be/models"
	"command-center-vms-cctv/be/utils"

	"github.com/joho/godotenv"
)

func main() {
	// Load environment variables
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found, using environment variables")
	}

	// Override DB_HOST for docker-compose
	if os.Getenv("DB_HOST") == "" || os.Getenv("DB_HOST") == "localhost" {
		// Try to connect to docker postgres
		os.Setenv("DB_HOST", "localhost")
		os.Setenv("DB_PORT", "5432")
		os.Setenv("DB_USER", "postgres")
		os.Setenv("DB_PASSWORD", "postgres")
		os.Setenv("DB_NAME", "vms_cctv")
		os.Setenv("DB_SSLMODE", "disable")
	}

	cfg := config.Load()
	db, err := database.Initialize(cfg.Database)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}

	// Check if admin user exists
	var user models.User
	if err := db.Where("email = ?", "admin@vms.demo").First(&user).Error; err != nil {
		// User doesn't exist, create it
		fmt.Println("Admin user not found, creating...")

		// Generate password hash for "demo123"
		hashedPassword, err := utils.HashPassword("demo123")
		if err != nil {
			log.Fatalf("Failed to hash password: %v", err)
		}

		// Create admin user
		admin := &models.User{
			Email:    "admin@vms.demo",
			Name:     "Admin User",
			Password: hashedPassword,
			Role:     "admin",
		}

		if err := db.Create(admin).Error; err != nil {
			log.Fatalf("Failed to create admin user: %v", err)
		}

		fmt.Println("✅ Admin user created successfully!")
		fmt.Println("   Email: admin@vms.demo")
		fmt.Println("   Password: demo123")
	} else {
		// User exists, reset password
		fmt.Println("Admin user found, resetting password...")

		// Generate new password hash for "demo123"
		hashedPassword, err := utils.HashPassword("demo123")
		if err != nil {
			log.Fatalf("Failed to hash password: %v", err)
		}

		user.Password = hashedPassword
		if err := db.Save(&user).Error; err != nil {
			log.Fatalf("Failed to update password: %v", err)
		}

		fmt.Println("✅ Admin password reset successfully!")
		fmt.Println("   Email: admin@vms.demo")
		fmt.Println("   Password: demo123")
	}
}
