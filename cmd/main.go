package main

import (
	"ai-routes-service/internal/grpc"
	"ai-routes-service/internal/handler"
	"ai-routes-service/internal/routes"
	"ai-routes-service/internal/services"
	"log"
	"os"

	
	"github.com/gofiber/fiber/v2"
)

var (
	//ApiKey    = os.Getenv("API_KEY")
	ApiKey ="AIzaSyBjPTWK9pkKdd5CB_TfY3FPpk6OA4hUk7g"
	//ModelName = os.Getenv("MODEL_NAME")
	//GoogleSearchKey = os.Getenv("GOOGLE_SEARCH_KEY")
	//GoogleSearchCX = os.Getenv("GOOGLE_SEARCH_CX")
	ModelName = "gemini-2.5-flash-lite-preview-06-17"
	GoogleSearchKey = "AIzaSyCvLCnXNTh__PjZi_aoyGeTCnZlrUcJkUk"
	GoogleSearchCX = "f5151badcb4504067"
	GRPCPort        = os.Getenv("GRPC_PORT")
	HTTPPort        = os.Getenv("HTTP_PORT")


	
)

func main() {
	app := fiber.New()

	aiService, err := services.NewAIService(ApiKey, ModelName, GoogleSearchKey, GoogleSearchCX)

	if err != nil {
		log.Fatalf("AI service initialization failed: %v", err)
	}

	go func() {
		if GRPCPort == "" {
			GRPCPort = "50051"
		}
		grpc.StartGRPCServer(aiService, GRPCPort)
	}()

	aiHandler := handler.NewAIHandler(aiService)

	routes.AIRoute(app, aiHandler)

	err = app.Listen(":9000")
	if err != nil {
		log.Fatalf("Sunucu başlatılamadı: %v", err)
	}
}
