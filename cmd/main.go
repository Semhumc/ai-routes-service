package main

import (
	"ai-routes-service/internal/grpc"
	"ai-routes-service/internal/handler"
	"ai-routes-service/internal/routes"
	"ai-routes-service/internal/services"
	"log"
	"os"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/logger"
)

var (
	// Environment variables - fallback değerlerle
	ApiKey          = getEnvOrDefault("API_KEY", "AIzaSyBjPTWK9pkKdd5CB_TfY3FPpk6OA4hUk7g")
	ModelName       = getEnvOrDefault("MODEL_NAME", "gemini-2.5-flash-lite-preview-06-17")
	GoogleSearchKey = getEnvOrDefault("GOOGLE_SEARCH_KEY", "AIzaSyCvLCnXNTh__PjZi_aoyGeTCnZlrUcJkUk")
	GoogleSearchCX  = getEnvOrDefault("GOOGLE_SEARCH_CX", "f5151badcb4504067")
	GRPCPort        = getEnvOrDefault("GRPC_PORT", "50051")
	HTTPPort        = getEnvOrDefault("HTTP_PORT", "9000")
)

func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func main() {
	log.Printf("🚀 AI Routes Service başlatılıyor...")
	log.Printf("📊 Config: GRPC Port: %s, HTTP Port: %s", GRPCPort, HTTPPort)

	// Fiber app oluştur
	app := fiber.New(fiber.Config{
		ErrorHandler: func(c *fiber.Ctx, err error) error {
			log.Printf("❌ Fiber Error: %v", err)
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "Internal Server Error",
			})
		},
	})

	// Middleware'ler
	app.Use(logger.New(logger.Config{
		Format: "🌐 ${time} | ${status} | ${latency} | ${method} ${path}\n",
	}))

	app.Use(cors.New(cors.Config{
		AllowOrigins:     "*",
		AllowMethods:     "GET,POST,PUT,DELETE,OPTIONS",
		AllowHeaders:     "Origin, Content-Type, Accept, Authorization",
		AllowCredentials: false,
	}))

	// AI Service initialize et
	aiService, err := services.NewAIService(ApiKey, ModelName, GoogleSearchKey, GoogleSearchCX)
	if err != nil {
		log.Fatalf("❌ AI service initialization failed: %v", err)
	}
	log.Printf("✅ AI Service başarıyla oluşturuldu")

	// gRPC Server'ı goroutine'de başlat
	go func() {
		log.Printf("🔧 gRPC Server başlatılıyor - Port: %s", GRPCPort)
		grpc.StartGRPCServer(aiService, GRPCPort)
	}()

	// HTTP Handler'ı oluştur
	aiHandler := handler.NewAIHandler(aiService)

	// Routes'ları kaydet
	routes.AIRoute(app, aiHandler)

	// Health check endpoint
	app.Get("/health", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{
			"status": "healthy",
			"service": "ai-routes-service",
		})
	})

	// HTTP Server'ı başlat
	log.Printf("🌐 HTTP Server başlatılıyor - Port: %s", HTTPPort)
	err = app.Listen(":" + HTTPPort)
	if err != nil {
		log.Fatalf("❌ HTTP Server başlatılamadı: %v", err)
	}
}