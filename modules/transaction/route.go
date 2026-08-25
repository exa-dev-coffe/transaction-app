package transaction

import (
	"log/slog"

	"eka-dev.cloud/transaction-service/lib"
	"eka-dev.cloud/transaction-service/middleware"
	"eka-dev.cloud/transaction-service/utils/common"
	"eka-dev.cloud/transaction-service/utils/response"
	"github.com/gofiber/fiber/v2"
	"github.com/jmoiron/sqlx"
)

type Handler interface {
	// TODO: define handler methods
	CreateTransaction(c *fiber.Ctx) error
	CreatePosTransaction(c *fiber.Ctx) error
	ChangePosPaymentMethod(c *fiber.Ctx) error
	SyncPosQrisStatus(c *fiber.Ctx) error
	GetListPosTransactions(c *fiber.Ctx) error
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

func NewHandler(app *fiber.App, service Service, db *sqlx.DB) Handler {
	h := &handler{service: service, db: db}

	routes := app.Group("/api/1.0")
	routes.Post("/checkout", middleware.RequireAuth, h.CreateTransaction)
	routes.Post("/pos/checkout", middleware.RequirePermission("pos", "create"), h.CreatePosTransaction)
	routes.Patch("/pos/transactions/:id/change-payment", middleware.RequirePermission("pos", "create"), h.ChangePosPaymentMethod)
	routes.Post("/pos/transactions/:id/sync-midtrans", middleware.RequirePermission("pos", "view"), h.SyncPosQrisStatus)
	routes.Get("/pos/transactions", middleware.RequirePermission("pos", "view"), h.GetListPosTransactions)
	routes.Get("/transactions", middleware.RequireAnyPermission(
		middleware.FeatureAction{Feature: "order", Action: "view"},
		middleware.FeatureAction{Feature: "report", Action: "view"},
	), h.GetListTransactions)
	routes.Get("/transactions/detail", middleware.RequireAnyPermission(
		middleware.FeatureAction{Feature: "order", Action: "view"},
		middleware.FeatureAction{Feature: "report", Action: "view"},
	), h.GetOneTransaction)
	routes.Get("/history-checkouts", middleware.RequireAuth, h.GetListTransactionsByUserId)
	routes.Get("/history-checkouts/detail", middleware.RequireAuth, h.GetOneTransactionByUserId)
	routes.Patch("/transactions/update-order-status", middleware.RequirePermission("order", "edit"), h.UpdateOrderStatus)
	routes.Patch("/history-checkouts/set-rating-menu", middleware.RequireAuth, h.SetRatingMenu)
	routes.Get("/transactions/summary-report", middleware.RequireAnyPermission(
		middleware.FeatureAction{Feature: "report", Action: "view"},
		middleware.FeatureAction{Feature: "order", Action: "view"},
	), h.SummaryReportTransactions)

	return h
}

func (h *handler) SyncPosQrisStatus(c *fiber.Ctx) error {
	id, err := c.ParamsInt("id")
	if err != nil {
		return response.BadRequest("Invalid transaction id", nil)
	}

	res, err := h.service.SyncPosQrisStatus(id)
	if err != nil {
		return err
	}

	return c.Status(fiber.StatusOK).JSON(response.Success("POS transaction status synced successfully", res))
}

func (h *handler) ChangePosPaymentMethod(c *fiber.Ctx) error {
	id, err := c.ParamsInt("id")
	if err != nil {
		return response.BadRequest("Invalid transaction id", nil)
	}

	var request ChangePosPaymentMethodRequest
	if err := c.BodyParser(&request); err != nil {
		slog.Error("Failed to parse change payment request body", "error", err)
		return response.BadRequest("Invalid request body", nil)
	}

	err = lib.ValidateRequest(request)
	if err != nil {
		return err
	}

	claims, err := common.GetClaimsFromLocals(c)
	if err == nil {
		request.UpdatedBy = claims.UserId
	}

	var result *TransactionResponse
	err = common.WithTransaction[ChangePosPaymentMethodRequest](h.db, func(tx *sqlx.Tx, req ChangePosPaymentMethodRequest) error {
		res, err := h.service.ChangePosPaymentMethod(tx, id, req)
		if err != nil {
			return err
		}
		result = res
		return nil
	}, request)
	if err != nil {
		return err
	}

	return c.Status(fiber.StatusOK).JSON(response.Success("Payment method updated successfully", result))
}

func (h *handler) CreatePosTransaction(c *fiber.Ctx) error {
	var request CreatePosTransactionRequest
	if err := c.BodyParser(&request); err != nil {
		slog.Error("Failed to parse POS request body", "error", err)
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

	var result *TransactionResponse
	err = common.WithTransaction[CreatePosTransactionRequest](h.db, func(tx *sqlx.Tx, req CreatePosTransactionRequest) error {
		res, err := h.service.CreatePosTransaction(tx, req)
		if err != nil {
			return err
		}
		result = res
		return nil
	}, request)
	if err != nil {
		return err
	}

	return c.Status(fiber.StatusCreated).JSON(response.Success("POS order created successfully", result))
}

func (h *handler) CreateTransaction(c *fiber.Ctx) error {
	// Parse request body
	var request CreateTransactionRequest
	if err := c.BodyParser(&request); err != nil {
		slog.Error("Failed to parse request body", "error", err)
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
			slog.Error("Failed to parse request query", "error", err)
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

func (h *handler) GetListPosTransactions(c *fiber.Ctx) error {
	queryParams := c.Queries()
	var paramsListRequest common.ParamsListRequest
	if err := common.ParseQueryParams(queryParams, &paramsListRequest); err != nil {
		return err
	}

	startDate := queryParams["startDate"]
	endDate := queryParams["endDate"]

	var request = GetListTransactionsRequest{
		ParamsListRequest: paramsListRequest,
		StartDate:         startDate,
		EndDate:           endDate,
	}

	err := lib.ValidateRequest(request)
	if err != nil {
		return err
	}

	request.ParamsListRequest.Search.Field = append(request.ParamsListRequest.Search.Field, "isCashier")
	request.ParamsListRequest.Search.Value = append(request.ParamsListRequest.Search.Value, "true")

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
		slog.Error("Failed to parse request body", "error", err)
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
		slog.Error("Failed to parse request body", "error", err)
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
		slog.Error("Failed to parse request query", "error", err)
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
