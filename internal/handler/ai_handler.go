package handler

import (
	"ai-routes-service/internal/models"
	"ai-routes-service/internal/services"
	"log"

	"github.com/gofiber/fiber/v2"
)

type AIHandler struct {
	AIService *services.AIService
}

type AIHandlerInterface interface {
	GenerateTripPlanHandler(c *fiber.Ctx) error
}

func NewAIHandler(aiService *services.AIService) *AIHandler {
	return &AIHandler{
		AIService: aiService,
	}
}

func (h *AIHandler) GenerateTripPlanHandler(c *fiber.Ctx) error {
	log.Printf("📥 AI Handler: Request alındı")
	
	req := c.Locals("req").(models.ReqBody)
	log.Printf("📋 AI Handler: Prompt data: %+v", req.Prompt)

	output, err := h.AIService.GenerateTripPlan(req.Prompt)
	if err != nil {
		log.Printf("❌ AI Handler: Service hatası: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	log.Printf("✅ AI Handler: Başarılı response")
	return c.JSON(fiber.Map{
		"result": output,
	})
}