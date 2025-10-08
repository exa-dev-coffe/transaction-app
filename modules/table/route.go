package table

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
	GetTables(c *fiber.Ctx) error
	CreateTable(c *fiber.Ctx) error
	UpdateTable(c *fiber.Ctx) error
	DeleteTable(c *fiber.Ctx) error
}

type handler struct {
	service Service
	db      *sqlx.DB
}

func NewHandler(app *fiber.App, db *sqlx.DB) Handler {
	repo := NewTableRepository(db)
	service := NewTableService(repo, db)
	handler := &handler{service: service, db: db}

	routes := app.Group("/api/1.0/tables")
	routes.Get("", middleware.RequireAuth, handler.GetTables)
	routes.Post("", middleware.RequireRole("admin"), handler.CreateTable)
	routes.Put("", middleware.RequireRole("admin"), handler.UpdateTable)
	routes.Delete("", middleware.RequireRole("admin"), handler.DeleteTable)

	return handler
}

func (h *handler) GetTables(c *fiber.Ctx) error {
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
		records, err = h.service.GetListTablesNoPagination(paramsListRequest)
	} else {
		records, err = h.service.GetListTablesPagination(paramsListRequest)
	}
	if err != nil {
		return err
	}
	return c.Status(fiber.StatusOK).JSON(response.Success("Success", records))
}

func (h *handler) CreateTable(c *fiber.Ctx) error {
	var request CreateTableRequest
	err := c.BodyParser(&request)
	if err != nil {
		return response.BadRequest("Invalid request body", err)
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

	err = common.WithTransaction[CreateTableRequest](h.db, h.service.InsertTable, request)
	if err != nil {
		return err
	}
	return c.Status(fiber.StatusCreated).JSON(response.Success("Table created successfully", nil))
}

func (h *handler) UpdateTable(c *fiber.Ctx) error {
	var request UpdateTableRequest
	err := c.BodyParser(&request)
	if err != nil {
		log.Error("Error parsing request body:", err)
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
	request.UpdatedBy = claims.UserId
	err = common.WithTransaction[UpdateTableRequest](h.db, h.service.UpdateTable, request)
	if err != nil {
		return err
	}
	return c.Status(fiber.StatusOK).JSON(response.Success("Table updated successfully", nil))
}

func (h *handler) DeleteTable(c *fiber.Ctx) error {
	requestId, err := common.GetOneDataRequest(c)
	if err != nil {
		return err
	}
	err = common.WithTransaction[int](h.db, h.service.DeleteTable, requestId.Id)
	if err != nil {
		return err
	}
	return c.Status(fiber.StatusOK).JSON(response.Success("Table deleted successfully", nil))
}
