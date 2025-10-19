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
	UpdateOrderStatus(c *fiber.Ctx) error
	SetRatingMenu(c *fiber.Ctx) error
	SummaryReportTransactions(c *fiber.Ctx) error
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
	routes.Patch("/transactions/update-order-status", middleware.RequireRole("admin", "barista"), h.UpdateOrderStatus)
	routes.Patch("/history-checkouts/set-rating-menu", middleware.RequireAuth, h.SetRatingMenu)
	routes.Get("/transactions/summary-report", middleware.RequireRole("admin", "barista"), h.SummaryReportTransactions)

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

	startDate := queryParams["startDate"]
	endDate := queryParams["endDate"]

	if startDate != "" && endDate != "" {
		dateRequest := common.DateOrder{}
		err := c.QueryParser(&dateRequest)
		if err != nil {
			log.Error("Failed to parse request query:", err)
			return response.BadRequest("Invalid request query", nil)
		}

		err = lib.ValidateRequest(dateRequest)
		if err != nil {
			return err
		}
	}

	var request = GetListTransactionsRequest{
		ParamsListRequest: paramsListRequest,
		StartDate:         startDate,
		EndDate:           endDate,
	}

	err := lib.ValidateRequest(request)
	if err != nil {
		return err
	}

	var records interface{}
	if paramsListRequest.NoPaginate {
		records, err = h.service.GetListTransactionsNoPagination(request)
	} else {
		records, err = h.service.GetListTransactionsPagination(request)
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

	records, err := h.service.GetListTransactionsByUserId(paramsListRequest, claims.UserId, claims.FullName)
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

	record, err := h.service.GetOneTransactionByUserId(request, claims.UserId, claims.FullName)
	if err != nil {
		return err
	}

	return c.Status(fiber.StatusOK).JSON(response.Success("Success", record))
}

func (h *handler) UpdateOrderStatus(c *fiber.Ctx) error {
	// Parse request body
	var request UpdateOrderStatusRequest
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

	request.UpdatedBy = claims.UserId

	err = common.WithTransaction[UpdateOrderStatusRequest](h.db, h.service.UpdateOrderStatus, request)
	if err != nil {
		return err
	}

	return c.Status(fiber.StatusOK).JSON(response.Success("Order status updated successfully", nil))
}

func (h *handler) SetRatingMenu(c *fiber.Ctx) error {
	// Parse request body
	var request SetRatingMenuRequest
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

	request.UpdatedBy = claims.UserId

	err = common.WithTransaction[SetRatingMenuRequest](h.db, h.service.SetRatingMenu, request)
	if err != nil {
		return err
	}

	return c.Status(fiber.StatusOK).JSON(response.Success("Set rating menu successfully", nil))
}

func (h *handler) SummaryReportTransactions(c *fiber.Ctx) error {
	// Parse query parameters

	var request common.DateOrder

	err := c.QueryParser(&request)
	if err != nil {
		log.Error("Failed to parse request query:", err)
		return response.BadRequest("Invalid request query", nil)
	}

	err = lib.ValidateRequest(request)
	if err != nil {
		return err
	}

	record, err := h.service.SummaryReportTransactions(request.StartDate, request.EndDate)
	if err != nil {
		return err
	}

	return c.Status(fiber.StatusOK).JSON(response.Success("Success", record))
}
