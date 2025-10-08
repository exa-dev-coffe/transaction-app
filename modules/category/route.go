package category

import (
	"eka-dev.cloud/transaction-service/lib"
	"eka-dev.cloud/transaction-service/middleware"
	"eka-dev.cloud/transaction-service/utils/common"
	"eka-dev.cloud/transaction-service/utils/response"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/log"
	"github.com/jmoiron/sqlx"
)

type Handler interface {
	GetCategories(c *fiber.Ctx) error
	CreateCategory(c *fiber.Ctx) error
	DeleteCategory(c *fiber.Ctx) error
}

type handler struct {
	service Service
	db      *sqlx.DB
}

// NewHandler return handler dan daftarin route
func NewHandler(app *fiber.App, db *sqlx.DB) Handler {
	repo := NewCategoryRepository(db)
	service := NewCategoryService(repo, db)
	handler := &handler{service: service, db: db}

	// mapping routes
	routes := app.Group("/api/1.0/categories")
	routes.Get("", handler.GetCategories)
	routes.Post("", middleware.RequireRole("admin"), handler.CreateCategory)
	routes.Delete("", middleware.RequireRole("admin"), handler.DeleteCategory)

	return handler
}

func (h *handler) GetCategories(c *fiber.Ctx) error {
	// parsing query params
	queryParams := c.Queries()
	var paramsListRequest common.ParamsListRequest
	err := common.ParseQueryParams(queryParams, &paramsListRequest)
	if err != nil {
		return err
	}

	err = lib.ValidateRequest(paramsListRequest)
	if err != nil {
		return err
	}

	var records interface{}
	if paramsListRequest.NoPaginate {
		records, err = h.service.GetListCategoriesNoPagination(paramsListRequest)
	} else {
		records, err = h.service.GetListCategoriesPagination(paramsListRequest)
	}

	if err != nil {
		return err
	}

	return c.Status(fiber.StatusOK).JSON(response.Success("Success", records))
}

func (h *handler) CreateCategory(c *fiber.Ctx) error {
	var request CreateCategoryRequest
	err := c.BodyParser(&request)
	if err != nil {
		log.Error("Error parsing request body: ", err)
		return response.BadRequest("Invalid request body", nil)
	}

	err = lib.ValidateRequest(request)
	if err != nil {
		return err
	}

	claims, err := common.GetClaimsFromLocals(c)
	if err != nil {
		return err
	}

	request.CreatedBy = claims.UserId

	err = common.WithTransaction[CreateCategoryRequest](h.db, h.service.InsertCategory, request)
	if err != nil {
		return err
	}

	return c.Status(fiber.StatusCreated).JSON(response.Success("Category created successfully", nil))
}

func (h *handler) DeleteCategory(c *fiber.Ctx) error {
	request, err := common.GetOneDataRequest(c)
	if err != nil {
		return err
	}

	err = common.WithTransaction[*common.OneRequest](h.db, h.service.DeleteCategory, request)
	if err != nil {
		return err
	}

	return c.Status(fiber.StatusOK).JSON(response.Success("Category deleted successfully", nil))
}
