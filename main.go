package main

import (
	"log"
	"os"

	"command-center-vms-cctv/be/config"
	"command-center-vms-cctv/be/database"
	"command-center-vms-cctv/be/handlers"
	"command-center-vms-cctv/be/middleware"
	"command-center-vms-cctv/be/services"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
)

func main() {
	// Load environment variables
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found, using environment variables")
	}

	// Load configuration
	cfg := config.Load()

	// Initialize database
	db, err := database.Initialize(cfg.Database)
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}

	// Initialize RTSP service
	rtspService := services.NewRTSPService(cfg.RTSP)

	// Initialize handlers
	authHandler := handlers.NewAuthHandler(db, cfg.JWT)
	cameraHandler := handlers.NewCameraHandler(db, rtspService)

	// Setup router
	router := setupRouter(authHandler, cameraHandler, cfg)

	// Start server
	port := cfg.Server.Port
	if port == "" {
		port = "8080"
	}

	log.Printf("Server starting on port %s", port)
	if err := router.Run(":" + port); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}

func setupRouter(authHandler *handlers.AuthHandler, cameraHandler *handlers.CameraHandler, cfg *config.Config) *gin.Engine {
	// Set Gin mode
	if os.Getenv("GIN_MODE") == "release" {
		gin.SetMode(gin.ReleaseMode)
	}

	router := gin.Default()

	// CORS configuration
	// Allow all localhost origins for development
	router.Use(cors.New(cors.Config{
		AllowOriginFunc: func(origin string) bool {
			// Allow requests with no origin (like mobile apps or curl requests)
			if origin == "" {
				return true
			}
			// Allow all localhost and 127.0.0.1 origins
			return origin == "http://localhost:8080" ||
				origin == "http://localhost:5173" ||
				origin == "http://localhost:3000" ||
				origin == "http://127.0.0.1:8080" ||
				origin == "http://127.0.0.1:5173" ||
				origin == "http://127.0.0.1:3000"
		},
		AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "OPTIONS", "PATCH"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Accept", "Authorization", "X-Requested-With"},
		ExposeHeaders:    []string{"Content-Length", "Content-Type"},
		AllowCredentials: true,
		MaxAge:           12 * 3600, // 12 hours
	}))

	// Health check
	router.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})

	// Serve HLS files statically
	router.Static(cfg.RTSP.StreamPath, cfg.RTSP.OutputPath)

	// Public routes
	api := router.Group("/api/v1")
	{
		// Auth routes
		auth := api.Group("/auth")
		{
			auth.POST("/login", authHandler.Login)
		}
	}

	// Protected routes
	protected := api.Group("")
	protected.Use(middleware.AuthMiddleware(cfg.JWT.Secret))
	{
		// Auth routes
		protected.GET("/auth/me", authHandler.GetMe)
		protected.POST("/auth/logout", authHandler.Logout)

		// Camera routes
		cameras := protected.Group("/cameras")
		{
			cameras.GET("", cameraHandler.GetCameras)
			cameras.GET("/:id", cameraHandler.GetCamera)
			cameras.POST("", cameraHandler.CreateCamera)
			cameras.PUT("/:id", cameraHandler.UpdateCamera)
			cameras.DELETE("/:id", cameraHandler.DeleteCamera)
			cameras.GET("/:id/stream", cameraHandler.GetStreamURL)
			cameras.GET("/:id/stream/health", cameraHandler.GetStreamHealth)
		}
	}

	return router
}

