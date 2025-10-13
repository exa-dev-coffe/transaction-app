package transaction

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
	// TODO: define handler methods
	CreateTransaction(c *fiber.Ctx) error
	GetListTransactions(c *fiber.Ctx) error
	GetOneTransaction(c *fiber.Ctx) error
	GetListTransactionsByUserId(c *fiber.Ctx) error
	GetOneTransactionByUserId(c *fiber.Ctx) error
}

type handler struct {
	service Service
	db      *sqlx.DB
}

func NewHandler(app *fiber.App, db *sqlx.DB) Handler {
	repo := NewTransactionRepository(db)
	service := NewTransactionService(repo, db)
	h := &handler{service: service, db: db}

	routes := app.Group("/api/1.0")
	routes.Post("/checkout", middleware.RequireAuth, h.CreateTransaction)
	routes.Get("/transactions", middleware.RequireRole("admin", "barista"), h.GetListTransactions)
	routes.Get("/transactions/detail", middleware.RequireRole("admin", "barista"), h.GetOneTransaction)
	routes.Get("/history-checkouts", middleware.RequireAuth, h.GetListTransactionsByUserId)
	routes.Get("/history-checkouts/detail", middleware.RequireAuth, h.GetOneTransactionByUserId)

	// routes.Get("", h.GetSomething)

	return h
}

func (h *handler) CreateTransaction(c *fiber.Ctx) error {
	// Parse request body
	var request CreateTransactionRequest
	if err := c.BodyParser(&request); err != nil {
		log.Error("Failed to parse request body:", err)
		return response.BadRequest("Invalid request body", nil)
	}

	err := lib.ValidateRequest(request)

	if err != nil {
		return err
	}

	claims, err := common.GetClaimsFromLocals(c)
	if err != nil {
		return err
	}

	request.CreatedBy = claims.UserId

	err = common.WithTransaction[CreateTransactionRequest](h.db, h.service.CreateTransaction, request)
	if err != nil {
		return err
	}

	return c.Status(fiber.StatusCreated).JSON(response.Success("Transaction created successfully", nil))
}

func (h *handler) GetListTransactions(c *fiber.Ctx) error {
	// Parse query parameters
	queryParams := c.Queries()
	var paramsListRequest common.ParamsListRequest
	if err := common.ParseQueryParams(queryParams, &paramsListRequest); err != nil {
		return err
	}

	err := lib.ValidateRequest(paramsListRequest)
	if err != nil {
		return err
	}

	var records interface{}
	if paramsListRequest.NoPaginate {
		records, err = h.service.GetListTransactionsNoPagination(paramsListRequest)
	} else {
		records, err = h.service.GetListTransactionsPagination(paramsListRequest)
	}
	if err != nil {
		return err
	}

	return c.Status(fiber.StatusOK).JSON(response.Success("Success", records))
}

func (h *handler) GetOneTransaction(c *fiber.Ctx) error {
	// Parse path parameter
	request, err := common.GetOneDataRequest(c)
	if err != nil {
		return err
	}

	record, err := h.service.GetOneTransaction(request)
	if err != nil {
		return err
	}

	return c.Status(fiber.StatusOK).JSON(response.Success("Success", record))
}

func (h *handler) GetListTransactionsByUserId(c *fiber.Ctx) error {
	// Parse query parameters
	queryParams := c.Queries()
	var paramsListRequest common.ParamsListRequest
	if err := common.ParseQueryParams(queryParams, &paramsListRequest); err != nil {
		return err
	}

	err := lib.ValidateRequest(paramsListRequest)
	if err != nil {
		return err
	}

	claims, err := common.GetClaimsFromLocals(c)
	if err != nil {
		return err
	}

	records, err := h.service.GetListTransactionsByUserId(paramsListRequest, claims.UserId)
	if err != nil {
		return err
	}

	return c.Status(fiber.StatusOK).JSON(response.Success("Success", records))
}

func (h *handler) GetOneTransactionByUserId(c *fiber.Ctx) error {
	// Parse path parameter
	request, err := common.GetOneDataRequest(c)
	if err != nil {
		return err
	}

	claims, err := common.GetClaimsFromLocals(c)
	if err != nil {
		return err
	}

	record, err := h.service.GetOneTransactionByUserId(request, claims.UserId)
	if err != nil {
		return err
	}

	return c.Status(fiber.StatusOK).JSON(response.Success("Success", record))
}
