package main

import (
	"fmt"
	"log"
	"os"

	"command-center-vms-cctv/be/config"
	"command-center-vms-cctv/be/database"
	"command-center-vms-cctv/be/models"
	"golang.org/x/crypto/bcrypt"
)

func main() {
	// Load environment variables
	if err := os.Setenv("DB_HOST", "localhost"); err != nil {
		log.Fatal(err)
	}
	if err := os.Setenv("DB_PORT", "5432"); err != nil {
		log.Fatal(err)
	}
	if err := os.Setenv("DB_USER", "postgres"); err != nil {
		log.Fatal(err)
	}
	if err := os.Setenv("DB_PASSWORD", "postgres"); err != nil {
		log.Fatal(err)
	}
	if err := os.Setenv("DB_NAME", "vms_cctv"); err != nil {
		log.Fatal(err)
	}
	if err := os.Setenv("DB_SSLMODE", "disable"); err != nil {
		log.Fatal(err)
	}

	cfg := config.Load()
	db, err := database.Initialize(cfg.Database)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}

	// Generate new password hash for "demo123"
	password := "demo123"
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		log.Fatalf("Failed to hash password: %v", err)
	}

	// Update admin user password
	var user models.User
	if err := db.Where("email = ?", "admin@vms.demo").First(&user).Error; err != nil {
		log.Fatalf("User not found: %v", err)
	}

	user.Password = string(hashedPassword)
	if err := db.Save(&user).Error; err != nil {
		log.Fatalf("Failed to update password: %v", err)
	}

	fmt.Printf("Password updated successfully for %s\n", user.Email)
	fmt.Printf("New password hash: %s\n", user.Password)
}

