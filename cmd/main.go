package main

import (
	"ai-routes-service/internal/handler"
	"ai-routes-service/internal/routes"
	"ai-routes-service/internal/services"
	"log"

	"github.com/gofiber/fiber/v2"
)

var (
	//ApiKey    = os.Getenv("API_KEY")
	//ModelName = os.Getenv("MODEL_NAME")
	//GoogleSearchKey = os.Getenv("GOOGLE_SEARCH_KEY")
	//GoogleSearchCX = os.Getenv("GOOGLE_SEARCH_CX")

	ApiKey          = "AIzaSyDoxoGFfrG8ndi-LSVLnl-iRJPjbRpcUUY"
	ModelName       = "gemini-2.5-flash-lite-preview-06-17"
	GoogleSearchKey = "AIzaSyBjPTWK9pkKdd5CB_TfY3FPpk6OA4hUk7g"
	GoogleSearchCX  = "f5151badcb4504067"
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
