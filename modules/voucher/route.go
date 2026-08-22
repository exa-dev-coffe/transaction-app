package voucher

import (
	"strconv"

	"eka-dev.cloud/transaction-service/lib"
	"eka-dev.cloud/transaction-service/middleware"
	"eka-dev.cloud/transaction-service/utils/common"
	"eka-dev.cloud/transaction-service/utils/response"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/log"
)

type Handler interface {
	ValidateVoucher(c *fiber.Ctx) error
	DeactivateVoucher(c *fiber.Ctx) error
	CreateVoucher(c *fiber.Ctx) error
	GetListVouchers(c *fiber.Ctx) error
	DeleteVoucher(c *fiber.Ctx) error
	UpdateVoucherStatus(c *fiber.Ctx) error
}

type handler struct {
	service Service
}

func NewHandler(app *fiber.App, service Service) Handler {
	h := &handler{service: service}

	routes := app.Group("/api/1.0")

	// Validate (Client side)
	routes.Post("/transactions/validate-voucher", middleware.RequireAuth, h.ValidateVoucher)

	// Internal Callback (Worker side)
	routes.Post("/internal/vouchers/deactivate", middleware.RequireInternalSecret, h.DeactivateVoucher)

	// Admin & User Vouchers
	routes.Post("/vouchers", middleware.RequireRole("admin"), h.CreateVoucher)
	routes.Get("/vouchers", middleware.RequireAuth, h.GetListVouchers)
	routes.Delete("/vouchers/:id", middleware.RequireRole("admin"), h.DeleteVoucher)
	routes.Patch("/vouchers/:id/status", middleware.RequireRole("admin"), h.UpdateVoucherStatus)

	return h
}

func (h *handler) ValidateVoucher(c *fiber.Ctx) error {
	var request ValidateVoucherRequest
	if err := c.BodyParser(&request); err != nil {
		log.Error("Failed to parse request body: ", err)
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

	res, err := h.service.ValidateVoucher(request, claims.UserId)
	if err != nil {
		return err
	}

	return c.Status(fiber.StatusOK).JSON(response.Success("Success", res))
}

func (h *handler) DeactivateVoucher(c *fiber.Ctx) error {
	var request struct {
		ID int64 `json:"id" validate:"required"`
	}
	if err := c.BodyParser(&request); err != nil {
		log.Error("Failed to parse request body: ", err)
		return response.BadRequest("Invalid request body", nil)
	}

	err := lib.ValidateRequest(request)
	if err != nil {
		return err
	}

	err = h.service.DeactivateVoucher(nil, request.ID)
	if err != nil {
		return err
	}

	return c.Status(fiber.StatusOK).JSON(response.Success("Voucher deactivated successfully", nil))
}

func (h *handler) CreateVoucher(c *fiber.Ctx) error {
	var request CreateVoucherRequest
	if err := c.BodyParser(&request); err != nil {
		log.Error("Failed to parse request body: ", err)
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

	id, err := h.service.CreateVoucher(nil, request)
	if err != nil {
		return err
	}

	return c.Status(fiber.StatusCreated).JSON(response.Success("Voucher created successfully", map[string]interface{}{"id": id}))
}

func (h *handler) GetListVouchers(c *fiber.Ctx) error {
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

	// SECURITY ENFORCEMENT:
	// Only Admin role can retrieve non-public (is_public = FALSE) secret vouchers.
	// For all non-admin users (Customer, Barista, etc.), the backend strictly forces is_public = TRUE at SQL level!
	isPublicOnly := claims.Role != "admin"

	res, err := h.service.GetListVouchers(paramsListRequest, isPublicOnly, claims.UserId)
	if err != nil {
		return err
	}

	return c.Status(fiber.StatusOK).JSON(response.Success("Success", res))
}

func (h *handler) DeleteVoucher(c *fiber.Ctx) error {
	idStr := c.Params("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		log.Error("Failed to parse ID: ", err)
		return response.BadRequest("Invalid voucher ID", nil)
	}

	err = h.service.DeleteVoucher(nil, id)
	if err != nil {
		return err
	}

	return c.Status(fiber.StatusOK).JSON(response.Success("Voucher deleted successfully", nil))
}

func (h *handler) UpdateVoucherStatus(c *fiber.Ctx) error {
	idStr := c.Params("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		log.Error("Failed to parse ID: ", err)
		return response.BadRequest("Invalid voucher ID", nil)
	}

	var request UpdateVoucherStatusRequest
	if err := c.BodyParser(&request); err != nil {
		log.Error("Failed to parse status body: ", err)
		return response.BadRequest("Invalid request body", nil)
	}

	err = h.service.UpdateVoucherStatus(nil, id, request.IsActive, request.IsPublic)
	if err != nil {
		return err
	}

	return c.Status(fiber.StatusOK).JSON(response.Success("Voucher status updated successfully", nil))
}
