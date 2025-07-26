package middleware

import (
	"ai-routes-service/internal/models"

	"github.com/gofiber/fiber/v2"
)

func AIMiddleware(c *fiber.Ctx) error {

	var req models.ReqBody

	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	c.Locals("req", req)

	return c.Next()
}
