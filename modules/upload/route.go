package upload

import (
	"eka-dev.cloud/transaction-service/lib"
	"eka-dev.cloud/transaction-service/middleware"
	"eka-dev.cloud/transaction-service/utils/common"
	"eka-dev.cloud/transaction-service/utils/response"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/log"
)

type Handler interface {
	UploadMenuFoto(c *fiber.Ctx) error
	DeleteMenuFoto(c *fiber.Ctx) error
}

type handler struct {
	service Service
}

// NewHandler return handler dan daftarin route
func NewHandler(app *fiber.App) Handler {
	service := NewUploadService()
	handler := &handler{service: service}

	// mapping routes
	routes := app.Group("/api/1.0/upload")
	routes.Post("/upload-menu", middleware.RequireRole("admin"), handler.UploadMenuFoto)
	routes.Delete("/delete-menu", middleware.RequireRole("admin"), handler.DeleteMenuFoto)

	return handler
}

func (s *handler) UploadMenuFoto(c *fiber.Ctx) error {
	// Parse the multipart form:
	form, err := c.MultipartForm()
	if err != nil {
		log.Error("Error parsing multipart form: ", err)
		return response.BadRequest("Failed to parse multipart form", nil)
	}

	files := form.File["file"]
	if len(files) == 0 {
		return response.BadRequest("No file is uploaded", nil)
	}

	// For simplicity, we handle only the first file

	fileHeader, err := lib.ValidateImageFile(files[0])
	if err != nil {
		return err
	}

	res, err := s.service.UploadMenuFoto(fileHeader)

	if err != nil {
		return err
	}

	return c.Status(fiber.StatusOK).JSON(response.Success("File uploaded successfully", res))
}

func (s *handler) DeleteMenuFoto(c *fiber.Ctx) error {
	var request common.DeleteImageRequest
	err := c.QueryParser(&request)
	if err != nil {
		log.Error("Error parsing request: ", err)
		return response.BadRequest("Invalid query parameters", nil)
	}

	err = lib.ValidateRequest(request)
	if err != nil {
		return err
	}

	err = s.service.DeleteMenuFoto(request.Url)
	if err != nil {
		return err
	}

	return c.Status(fiber.StatusOK).JSON(response.Success("File deleted successfully", nil))
}
