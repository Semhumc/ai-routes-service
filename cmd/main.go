package main

import (
	"ai-routes-service/internal/handler"
	"ai-routes-service/internal/routes"
	"ai-routes-service/internal/services"
	"log"
	"os"

	"github.com/gofiber/fiber/v2"
)

var (
	ApiKey    = os.Getenv("API_KEY")
	ModelName = os.Getenv("MODEL_NAME")
	GoogleSearchKey = os.Getenv("GOOGLE_SEARCH_KEY")
	GoogleSearchCX = os.Getenv("GOOGLE_SEARCH_CX")

	
)

func main() {
	app := fiber.New()

	aiService, err := services.NewAIService(ApiKey, ModelName, GoogleSearchKey, GoogleSearchCX)

	if err != nil {
		log.Fatalf("AI service initialization failed: %v", err)
	}

	aiHandler := handler.NewAIHandler(aiService)

	routes.AIRoute(app, aiHandler)

	err = app.Listen(":9000")
	if err != nil {
		log.Fatalf("Sunucu başlatılamadı: %v", err)
	}
}
