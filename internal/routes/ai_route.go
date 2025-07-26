package routes

import (
	"ai-routes-service/internal/handler"
	"ai-routes-service/internal/middleware"

	"github.com/gofiber/fiber/v2"
)

func AIRoute(router fiber.Router, aiHandler handler.AIHandlerInterface){

	api := router.Group("/api/v1")

	api.Post("/ai",middleware.AIMiddleware,aiHandler.GenerateTripPlanHandler)

}