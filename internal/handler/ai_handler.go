package handler

import (
	"ai-routes-service/internal/models"
	"ai-routes-service/internal/services"

	"github.com/gofiber/fiber/v2"
)

type AIHandler struct {
	AIService *services.AIService
}

type AIHandlerInterface interface{
	GenerateTripPlanHandler(c *fiber.Ctx) error
}

func NewAIHandler(aiService *services.AIService) *AIHandler {
	return &AIHandler{
		AIService: aiService,
	}
}



func (h *AIHandler) GenerateTripPlanHandler(c *fiber.Ctx) error {

	req := c.Locals("req").(models.ReqBody)

	output, err := h.AIService.GenerateTripPlan(req.Prompt)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"result": output,
	})

}
