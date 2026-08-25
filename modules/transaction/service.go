package transaction

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/url"
	"strconv"
	"strings"
	"time"

	"eka-dev.cloud/transaction-service/config"
	"eka-dev.cloud/transaction-service/lib"
	"eka-dev.cloud/transaction-service/modules/voucher"
	"eka-dev.cloud/transaction-service/utils"
	"eka-dev.cloud/transaction-service/utils/common"
	"eka-dev.cloud/transaction-service/utils/response"
	"github.com/jmoiron/sqlx"
	amqp "github.com/rabbitmq/amqp091-go"
)

type Service interface {
	// TODO: define service methods
	CreateTransaction(tx *sqlx.Tx, request CreateTransactionRequest) error
	CreatePosTransaction(tx *sqlx.Tx, request CreatePosTransactionRequest) (*TransactionResponse, error)
	ChangePosPaymentMethod(tx *sqlx.Tx, id int, request ChangePosPaymentMethodRequest) (*TransactionResponse, error)
	SyncPosQrisStatus(id int) (*TransactionResponse, error)
	SettlePosQrisPayment(orderRef string, status string) (*TransactionResponse, error)
	GetListTransactionsPagination(request GetListTransactionsRequest) (*response.Pagination[[]TransactionResponse], error)
	GetListTransactionsNoPagination(request GetListTransactionsRequest) ([]TransactionResponse, error)
	GetOneTransaction(request *common.OneRequest) (*TransactionResponse, error)
	GetListTransactionsByUserId(request common.ParamsListRequest, userId int64, name string) (*response.Pagination[[]TransactionResponse], error)
	GetOneTransactionByUserId(request *common.OneRequest, userId int64, name string) (*TransactionResponse, error)
	UpdateOrderStatus(tx *sqlx.Tx, request UpdateOrderStatusRequest) error
	SetRatingMenu(tx *sqlx.Tx, request SetRatingMenuRequest) error
	SummaryReportTransactions(startDate string, endDate string) (*SummaryReportData, error)
}

type transactionService struct {
	repo           Repository
	voucherService voucher.Service
	db             *sqlx.DB
}

func NewTransactionService(repo Repository, voucherService voucher.Service, db *sqlx.DB) Service {
	return &transactionService{repo: repo, voucherService: voucherService, db: db}
}

func (s *transactionService) CreateTransaction(tx *sqlx.Tx, request CreateTransactionRequest) error {
	// Convert menuIds slice to a comma-separated string
	var ids string
	for i, data := range request.Datas {
		if i > 0 {
			ids += ","
		}
		ids += fmt.Sprintf("%d", data.MenuID)
	}
	menus, err := getAvailableMenuByIdsAndTableById(ids, request.TableId)
	if err != nil {
		return err
	}

	if len(menus) != len(request.Datas) {
		return response.BadRequest("No menus found for the given IDs", nil)
	}

	request.Total = calculateTotalPriceMenu(menus, &request)

	var voucherId *int64 = nil
	var discountAmount float64 = 0

	if request.VoucherCode != "" {
		vId, discount, err := s.voucherService.ValidateVoucherForCheckout(tx, request.VoucherCode, request.Total, request.CreatedBy)
		if err != nil {
			return err
		}
		discountAmount = discount
		voucherId = &vId
		request.Total -= discountAmount
	}

	// 1. Insert Header & Details into DB Transaction FIRST
	id, err := s.repo.InsertThTransaction(tx, request, voucherId, discountAmount)
	if err != nil {
		return err
	}

	if voucherId != nil {
		err = s.voucherService.LogVoucherUsage(tx, request.CreatedBy, *voucherId, int64(id), discountAmount)
		if err != nil {
			return err
		}
	}

	// Bulk Batch Write for Transaction Details
	err = s.repo.InsertTdTransactionBatch(tx, id, request.CreatedBy, request.Datas)
	if err != nil {
		return err
	}

	// Prepare Bulk Batch Write for Promotion Usage Logs
	promoLogs := make([]PromotionUsageLog, 0)
	for _, m := range menus {
		if m.Discount != nil && m.Discount.Savings > 0 {
			for _, data := range request.Datas {
				if m.Id == data.MenuID {
					promoLogs = append(promoLogs, PromotionUsageLog{
						TransactionID:  int64(id),
						PromotionID:    m.Discount.PromotionID,
						MenuID:         int64(m.Id),
						UserID:         request.CreatedBy,
						Qty:            data.Qty,
						DiscountAmount: m.Discount.Savings * float64(data.Qty),
					})
				}
			}
		}
	}

	if len(promoLogs) > 0 {
		_ = s.repo.LogPromotionUsageBatch(tx, promoLogs)
	}

	// 2. Perform Wallet Payment AFTER DB records are established in transaction
	err = paymentUseWallet(request.CreatedBy, request.Total, request.Pin)
	if err != nil {
		return err
	}

	// Trigger notifications asynchronously after successful creation
	go func() {
		// Wait a small duration to ensure DB transaction is committed
		time.Sleep(50 * time.Millisecond)

		// Fetch full transaction details (including menu names, table name, user name)
		orderDetail, err := s.GetOneTransaction(&common.OneRequest{Id: id})
		if err != nil {
			slog.Error("Failed to fetch full order details for SSE new order", "error", err)
			return
		}

		// 1. Get user details for email template
		var userName, email string
		users, err := getUsersNameByIds(fmt.Sprintf("%d", request.CreatedBy))
		if err == nil && len(users) > 0 {
			userName = users[0].FullName
			email = users[0].Email
		} else {
			userName = orderDetail.OrderBy
			email = ""
		}

		ch, err := lib.GetChannel()
		if err != nil {
			slog.Error("Failed to get rabbitmq channel for transaction notifications", "error", err)
			return
		}
		defer ch.Close()

		// 2. Publish SSE event for Barista real-time updates (Fanout exchange "order.created")
		type ssePayloadObj struct {
			Event string               `json:"event"`
			Data  *TransactionResponse `json:"data"`
		}
		payloadObj := ssePayloadObj{
			Event: "new_order",
			Data:  orderDetail,
		}
		ssePayload, err := json.Marshal(payloadObj)
		if err != nil {
			slog.Error("Failed to marshal SSE payload", "error", err)
		} else {
			err = lib.SendMessage(ch, "", "", "order.created", lib.ExchangeFanout, amqp.Publishing{
				ContentType: "application/json",
				Body:        ssePayload,
			}, string(ssePayload), false, false, true, amqp.Table{})
			if err != nil {
				slog.Error("Failed to publish SSE new order message", "error", err)
			}
		}

		// 3. Publish Email notification if user email exists (Direct exchange "email.queue")
		if email != "" {
			emailPayload := []byte(fmt.Sprintf(`{
				"to": "%s",
				"subject": "Order Confirmation - Diskusi Coffee",
				"userName": "%s",
				"orderId": %d,
				"orderFor": "%s",
				"amount": %f,
				"date": "%s"
			}`, email, userName, id, request.OrderFor, request.Total, time.Now().Format("2006-01-02 15:04:05")))

			err = lib.SendMessage(ch, "Email Order Receipt", "emailQueue.orderReceipt", "email.queue", lib.ExchangeDirect, amqp.Publishing{
				ContentType: "application/json",
				Body:        emailPayload,
			}, string(emailPayload), true, false, false, amqp.Table{})
			if err != nil {
				slog.Error("Failed to publish Email order receipt message", "error", err)
			}
		}
	}()

	return nil
}

func (s *transactionService) CreatePosTransaction(tx *sqlx.Tx, request CreatePosTransactionRequest) (*TransactionResponse, error) {
	// Convert menuIds slice to a comma-separated string
	var ids string
	for i, data := range request.Datas {
		if i > 0 {
			ids += ","
		}
		ids += fmt.Sprintf("%d", data.MenuID)
	}

	if strings.EqualFold(request.OrderType, "DINE_IN") && (request.TableId == nil || *request.TableId <= 0) {
		return nil, response.BadRequest("Table selection is required for DINE_IN orders", nil)
	}

	var tableId int64 = 0
	if request.TableId != nil {
		tableId = *request.TableId
	}

	menus, err := getAvailableMenuByIdsAndTableById(ids, tableId)
	if err != nil {
		return nil, err
	}

	if len(menus) != len(request.Datas) {
		return nil, response.BadRequest("No menus found for the given IDs", nil)
	}

	// Calculate total price
	var total float64
	for _, menu := range menus {
		itemPrice := menu.Price
		if menu.EffectivePrice > 0 {
			itemPrice = menu.EffectivePrice
		}
		for iD, data := range request.Datas {
			if menu.Id == data.MenuID {
				request.Datas[iD].Price = itemPrice
				request.Datas[iD].Total = itemPrice * float64(data.Qty)
				total += itemPrice * float64(data.Qty)
			}
		}
	}
	request.Total = total

	var voucherId *int64 = nil
	var discountAmount float64 = 0

	if request.VoucherCode != "" {
		vId, discount, err := s.voucherService.ValidateVoucherForCheckout(tx, request.VoucherCode, request.Total, request.CreatedBy)
		if err != nil {
			return nil, err
		}
		discountAmount = discount
		voucherId = &vId
		request.Total -= discountAmount
	}

	paymentStatus := "PAID"
	var qrString, qrUrl string

	if strings.EqualFold(request.PaymentMethod, "CASH") {
		if request.CashAmount < request.Total {
			return nil, response.BadRequest(fmt.Sprintf("Insufficient cash amount. Total is %.2f, received %.2f", request.Total, request.CashAmount), nil)
		}
		request.CashChange = request.CashAmount - request.Total
	} else if strings.EqualFold(request.PaymentMethod, "WALLET") {
		if strings.TrimSpace(request.WalletPaymentCode) == "" {
			return nil, response.BadRequest("Customer Wallet Payment Code is required", nil)
		}
	} else if strings.EqualFold(request.PaymentMethod, "MIDTRANS") {
		paymentStatus = "PENDING"
	}

	// 1. Insert Header & Details into DB Transaction FIRST
	id, err := s.repo.InsertThPosTransaction(tx, request, voucherId, discountAmount, paymentStatus)
	if err != nil {
		return nil, err
	}

	if voucherId != nil {
		err = s.voucherService.LogVoucherUsage(tx, request.CreatedBy, *voucherId, int64(id), discountAmount)
		if err != nil {
			return nil, err
		}
	}

	// Bulk Batch Write for Transaction Details
	err = s.repo.InsertTdTransactionBatch(tx, id, request.CreatedBy, request.Datas)
	if err != nil {
		return nil, err
	}

	// Prepare Bulk Batch Write for Promotion Usage Logs
	promoLogs := make([]PromotionUsageLog, 0)
	for _, m := range menus {
		if m.Discount != nil && m.Discount.Savings > 0 {
			for _, data := range request.Datas {
				if m.Id == data.MenuID {
					promoLogs = append(promoLogs, PromotionUsageLog{
						TransactionID:  int64(id),
						PromotionID:    m.Discount.PromotionID,
						MenuID:         int64(m.Id),
						UserID:         request.CreatedBy,
						Qty:            data.Qty,
						DiscountAmount: m.Discount.Savings * float64(data.Qty),
					})
				}
			}
		}
	}

	if len(promoLogs) > 0 {
		_ = s.repo.LogPromotionUsageBatch(tx, promoLogs)
	}

	// 2. Execute External Payment Processing AFTER DB record is established in transaction
	if strings.EqualFold(request.PaymentMethod, "WALLET") {
		walletRes, err := payPosWallet(request.WalletPaymentCode, request.Total, int64(id))
		if err != nil {
			return nil, err
		}
		if walletRes.UserId > 0 {
			request.CreatedBy = walletRes.UserId
		}
		if walletRes.CustomerName != "" && (request.OrderFor == "" || request.OrderFor == "Walk-in Guest") {
			request.OrderFor = walletRes.CustomerName
		}
		if walletRes.UserId > 0 || walletRes.CustomerName != "" {
			_ = s.repo.UpdatePosWalletCustomer(tx, id, request.CreatedBy, request.OrderFor)
		}
	} else if strings.EqualFold(request.PaymentMethod, "MIDTRANS") {
		posOrderRef := fmt.Sprintf("POS-%d", id)
		qrisRes, err := chargePosQris(posOrderRef, request.Total, request.OrderFor, "")
		if err != nil {
			return nil, err
		}
		qrString = qrisRes.QrString
		qrUrl = qrisRes.QrUrl
		_ = s.repo.UpdatePosQrisData(tx, id, qrString, qrUrl)
	}

	// Trigger notifications asynchronously if already paid (CASH or WALLET)
	if paymentStatus == "PAID" {
		go func() {
			time.Sleep(50 * time.Millisecond)

			orderDetail, err := s.GetOneTransaction(&common.OneRequest{Id: id})
			if err != nil {
				slog.Error("Failed to fetch full order details for SSE new order", "error", err)
				return
			}

			ch, err := lib.GetChannel()
			if err != nil {
				slog.Error("Failed to get rabbitmq channel for transaction notifications", "error", err)
				return
			}
			defer ch.Close()

			type ssePayloadObj struct {
				Event string               `json:"event"`
				Data  *TransactionResponse `json:"data"`
			}
			payloadObj := ssePayloadObj{
				Event: "new_order",
				Data:  orderDetail,
			}
			ssePayload, err := json.Marshal(payloadObj)
			if err != nil {
				slog.Error("Failed to marshal SSE payload", "error", err)
			} else {
				err = lib.SendMessage(ch, "", "", "order.created", lib.ExchangeFanout, amqp.Publishing{
					ContentType: "application/json",
					Body:        ssePayload,
				}, string(ssePayload), false, false, true, amqp.Table{})
				if err != nil {
					slog.Error("Failed to publish SSE new order message", "error", err)
				}
			}
		}()
	}

	res := &TransactionResponse{
		Id:             int64(id),
		TotalPrice:     request.Total,
		OrderFor:       request.OrderFor,
		OrderType:      request.OrderType,
		PaymentMethod:  request.PaymentMethod,
		PaymentStatus:  paymentStatus,
		CashAmount:     request.CashAmount,
		CashChange:     request.CashChange,
		IsCashier:      true,
		DiscountAmount: discountAmount,
		VoucherCode:    request.VoucherCode,
		QrString:       qrString,
		QrUrl:          qrUrl,
	}
	if request.TableId != nil {
		res.TableId = *request.TableId
	}
	return res, nil
}

func (s *transactionService) SettlePosQrisPayment(orderRef string, status string) (*TransactionResponse, error) {
	if !strings.HasPrefix(orderRef, "POS-") {
		return nil, response.BadRequest("Invalid POS order reference", nil)
	}

	parts := strings.Split(orderRef, "-")
	if len(parts) < 2 {
		return nil, response.BadRequest("Malformed POS order reference", nil)
	}

	id, err := strconv.Atoi(parts[1])
	if err != nil {
		return nil, response.BadRequest("Invalid transaction ID in POS order reference", nil)
	}

	if strings.EqualFold(status, "PAID") || strings.EqualFold(status, "COMPLETED") || strings.EqualFold(status, "SETTLEMENT") {
		return s.markOrderAsPaidAndNotify(id)
	}

	return s.SyncPosQrisStatus(id)
}

func (s *transactionService) notifyOrderPaidAsync(orderDetail *TransactionResponse) {
	if orderDetail == nil {
		return
	}
	go func() {
		ch, err := lib.GetChannel()
		if err != nil {
			slog.Error("Failed to get rabbitmq channel for transaction notifications", "error", err)
			return
		}
		defer ch.Close()

		type ssePayloadObj struct {
			Event string               `json:"event"`
			Data  *TransactionResponse `json:"data"`
		}
		payloadObj := ssePayloadObj{
			Event: "new_order",
			Data:  orderDetail,
		}
		ssePayload, err := json.Marshal(payloadObj)
		if err == nil {
			_ = lib.SendMessage(ch, "", "", "order.created", lib.ExchangeFanout, amqp.Publishing{
				ContentType: "application/json",
				Body:        ssePayload,
			}, string(ssePayload), false, false, true, amqp.Table{})
		}
	}()
}

func (s *transactionService) markOrderAsPaidAndNotify(id int) (*TransactionResponse, error) {
	orderDetail, err := s.GetOneTransaction(&common.OneRequest{Id: id})
	if err != nil {
		return nil, err
	}

	if strings.EqualFold(orderDetail.PaymentStatus, "PAID") {
		return orderDetail, nil
	}

	err = s.repo.UpdatePaymentStatus(nil, id, "PAID")
	if err != nil {
		return nil, err
	}
	orderDetail.PaymentStatus = "PAID"

	s.notifyOrderPaidAsync(orderDetail)

	return orderDetail, nil
}

func (s *transactionService) SyncPosQrisStatus(id int) (*TransactionResponse, error) {
	orderDetail, err := s.GetOneTransaction(&common.OneRequest{Id: id})
	if err != nil {
		return nil, err
	}

	if strings.EqualFold(orderDetail.PaymentStatus, "PAID") {
		return orderDetail, nil
	}

	// 1. Verify status directly with payment gateway via wallet-service
	posOrderRef := fmt.Sprintf("POS-%d", id)
	statusRes, err := checkPosQrisStatus(posOrderRef)
	if err != nil {
		slog.Error("Failed to check POS QRIS status with wallet-service", "orderId", posOrderRef, "error", err)
		return nil, response.BadRequest("Unable to verify payment status with payment gateway", nil)
	}

	var txStatus string
	if dataMap, ok := statusRes["data"].(map[string]interface{}); ok {
		if st, ok := dataMap["transactionStatus"].(string); ok {
			txStatus = st
		} else if st, ok := dataMap["transaction_status"].(string); ok {
			txStatus = st
		}
	}
	if txStatus == "" {
		if st, ok := statusRes["transaction_status"].(string); ok {
			txStatus = st
		} else if st, ok := statusRes["transactionStatus"].(string); ok {
			txStatus = st
		}
	}

	if !strings.EqualFold(txStatus, "settlement") && !strings.EqualFold(txStatus, "capture") && !strings.EqualFold(txStatus, "paid") {
		return nil, response.BadRequest(fmt.Sprintf("Payment is not settled yet. Current status: %s", txStatus), nil)
	}

	return s.markOrderAsPaidAndNotify(id)
}

func (s *transactionService) ChangePosPaymentMethod(tx *sqlx.Tx, id int, request ChangePosPaymentMethodRequest) (*TransactionResponse, error) {
	// 1. Fetch & Verify Order State
	orderDetail, err := s.GetOneTransaction(&common.OneRequest{Id: id})
	if err != nil {
		return nil, err
	}

	if strings.EqualFold(orderDetail.PaymentStatus, "PAID") {
		return orderDetail, nil
	}

	var cashAmount, cashChange float64
	var newUserId int64 = orderDetail.UserId
	var newOrderFor string = orderDetail.OrderFor

	if strings.EqualFold(request.PaymentMethod, "CASH") {
		if request.CashAmount < orderDetail.TotalPrice {
			return nil, response.BadRequest("Cash amount is less than order total price", nil)
		}
		cashAmount = request.CashAmount
		cashChange = request.CashChange

		err = s.repo.UpdatePosPaymentMethod(tx, id, "CASH", "PAID", cashAmount, cashChange)
		if err != nil {
			return nil, err
		}
	} else if strings.EqualFold(request.PaymentMethod, "WALLET") {
		if len(request.WalletPaymentCode) != 6 {
			return nil, response.BadRequest("Payment code must be 6 digits", nil)
		}

		walletRes, err := payPosWallet(request.WalletPaymentCode, orderDetail.TotalPrice, int64(id))
		if err != nil {
			return nil, err
		}

		var success bool
		defer func() {
			if !success && walletRes != nil && walletRes.UserId > 0 {
				slog.Error("POS transaction DB update failed after wallet payment. Triggering auto-refund", "orderId", id, "userId", walletRes.UserId)
				_ = refundPosWallet(walletRes.UserId, orderDetail.TotalPrice, int64(id))
			}
		}()

		newUserId = walletRes.UserId
		if walletRes.CustomerName != "" && (newOrderFor == "" || newOrderFor == "Walk-in Guest") {
			newOrderFor = walletRes.CustomerName
		}
		if newUserId > 0 || newOrderFor != orderDetail.OrderFor {
			err = s.repo.UpdatePosWalletCustomer(tx, id, newUserId, newOrderFor)
			if err != nil {
				return nil, err
			}
		}

		err = s.repo.UpdatePosPaymentMethod(tx, id, "WALLET", "PAID", 0, 0)
		if err != nil {
			return nil, err
		}

		success = true
	} else {
		return nil, response.BadRequest("Invalid payment method for change payment", nil)
	}

	orderDetail.PaymentMethod = request.PaymentMethod
	orderDetail.PaymentStatus = "PAID"
	orderDetail.CashAmount = cashAmount
	orderDetail.CashChange = cashChange
	orderDetail.UserId = newUserId
	orderDetail.OrderFor = newOrderFor

	s.notifyOrderPaidAsync(orderDetail)
	return orderDetail, nil
}

func (s *transactionService) GetListTransactionsPagination(request GetListTransactionsRequest) (*response.Pagination[[]TransactionResponse], error) {
	res, err := s.repo.GetListTransactionsPagination(request.ParamsListRequest, request.StartDate, request.EndDate)
	if err != nil {
		return nil, err
	}

	menuIds := []string{}
	tableIds := []string{}
	userIds := []string{}
	for _, data := range res.Data {
		tableIdStr := utils.Int64ToString(data.TableId)

		if data.TableId != 0 && !strings.Contains(strings.Join(tableIds, ","), tableIdStr) {
			tableIds = append(tableIds, tableIdStr)
		}

		if data.UserId != 0 && !strings.Contains(strings.Join(userIds, ","), utils.Int64ToString(data.UserId)) {
			userIds = append(userIds, utils.Int64ToString(data.UserId))
		}

		for _, detail := range data.Details {
			menuIdStr := utils.IntToString(detail.MenuId)
			if detail.MenuId != 0 && !strings.Contains(strings.Join(menuIds, ","), menuIdStr) {
				menuIds = append(menuIds, menuIdStr)
			}
		}
	}

	menuIdsStr := strings.Join(menuIds, ",")
	tableIdsStr := strings.Join(tableIds, ",")
	if tableIdsStr == "" {
		tableIdsStr = "0"
	}
	userIdsStr := strings.Join(userIds, ",")

	if menuIdsStr != "" && userIdsStr != "" {
		dataMenusAndTable, err := getDataMenuByIdsAndTable(menuIdsStr, tableIdsStr)
		if err != nil {
			return nil, err
		}
		dataUsers, err := getUsersNameByIds(userIdsStr)
		if err != nil {
			return nil, err
		}

		for i, data := range res.Data {
			for idDataDetail, dataDetail := range data.Details {
				for _, menu := range dataMenusAndTable.Menus {
					if dataDetail.MenuId == menu.Id {
						res.Data[i].Details[idDataDetail].MenuName = menu.Name
						res.Data[i].Details[idDataDetail].Photo = menu.Photo
						res.Data[i].Details[idDataDetail].Description = menu.Description
						break
					}
				}
			}
			for _, table := range dataMenusAndTable.Tables {
				if data.TableId == table.Id {
					res.Data[i].TableName = table.Name
					break
				}
			}
			if res.Data[i].TableName == "" {
				if strings.EqualFold(res.Data[i].OrderType, "TAKEAWAY") {
					res.Data[i].TableName = "Takeaway"
				} else if res.Data[i].TableId == 0 {
					res.Data[i].TableName = "No Table"
				}
			}
			for _, user := range dataUsers {
				if data.UserId == user.UserId {
					res.Data[i].OrderBy = user.FullName
					break
				}
			}
		}
	} else {
		for i := range res.Data {
			if res.Data[i].TableName == "" {
				if strings.EqualFold(res.Data[i].OrderType, "TAKEAWAY") {
					res.Data[i].TableName = "Takeaway"
				} else if res.Data[i].TableId == 0 {
					res.Data[i].TableName = "No Table"
				}
			}
		}
	}
	return res, nil
}

func (s *transactionService) GetListTransactionsNoPagination(request GetListTransactionsRequest) ([]TransactionResponse, error) {
	res, err := s.repo.GetListTransactionsNoPagination(request.ParamsListRequest, request.StartDate, request.EndDate)
	if err != nil {
		return nil, err
	}

	menuIds := []string{}
	tableIds := []string{}
	userIds := []string{}
	for _, data := range res {
		tableIdStr := utils.Int64ToString(data.TableId)

		if data.TableId != 0 && !strings.Contains(strings.Join(tableIds, ","), tableIdStr) {
			tableIds = append(tableIds, tableIdStr)
		}

		if data.UserId != 0 && !strings.Contains(strings.Join(userIds, ","), utils.Int64ToString(data.UserId)) {
			userIds = append(userIds, utils.Int64ToString(data.UserId))
		}

		for _, detail := range data.Details {
			menuIdStr := utils.IntToString(detail.MenuId)
			if detail.MenuId != 0 && !strings.Contains(strings.Join(menuIds, ","), menuIdStr) {
				menuIds = append(menuIds, menuIdStr)
			}
		}
	}

	menuIdsStr := strings.Join(menuIds, ",")
	tableIdsStr := strings.Join(tableIds, ",")
	if tableIdsStr == "" {
		tableIdsStr = "0"
	}
	userIdsStr := strings.Join(userIds, ",")

	if menuIdsStr != "" && userIdsStr != "" {
		dataMenusAndTable, err := getDataMenuByIdsAndTable(menuIdsStr, tableIdsStr)
		if err != nil {
			return nil, err
		}
		dataUsers, err := getUsersNameByIds(userIdsStr)
		if err != nil {
			return nil, err
		}

		for i, data := range res {
			for idDataDetail, dataDetail := range data.Details {
				for _, menu := range dataMenusAndTable.Menus {
					if dataDetail.MenuId == menu.Id {
						res[i].Details[idDataDetail].MenuName = menu.Name
						res[i].Details[idDataDetail].Photo = menu.Photo
						res[i].Details[idDataDetail].Description = menu.Description
						break
					}
				}
			}
			for _, table := range dataMenusAndTable.Tables {
				if data.TableId == table.Id {
					res[i].TableName = table.Name
					break
				}
			}
			if res[i].TableName == "" {
				if strings.EqualFold(res[i].OrderType, "TAKEAWAY") {
					res[i].TableName = "Takeaway"
				} else if res[i].TableId == 0 {
					res[i].TableName = "No Table"
				}
			}

			for _, user := range dataUsers {
				if data.UserId == user.UserId {
					res[i].OrderBy = user.FullName
					break
				}
			}
		}
	} else {
		for i := range res {
			if res[i].TableName == "" {
				if strings.EqualFold(res[i].OrderType, "TAKEAWAY") {
					res[i].TableName = "Takeaway"
				} else if res[i].TableId == 0 {
					res[i].TableName = "No Table"
				}
			}
		}
	}

	return res, nil
}

func (s *transactionService) GetOneTransaction(request *common.OneRequest) (*TransactionResponse, error) {
	res, err := s.repo.GetOneTransaction(request.Id)
	if err != nil {
		return nil, err
	}

	menuIds := []string{}
	tableIdStr := utils.Int64ToString(res.TableId)
	if tableIdStr == "" || tableIdStr == "0" {
		tableIdStr = "0"
	}
	userIdStr := utils.Int64ToString(res.UserId)

	for _, detail := range res.Details {
		menuIdStr := utils.IntToString(detail.MenuId)
		if detail.MenuId != 0 && !strings.Contains(strings.Join(menuIds, ","), menuIdStr) {
			menuIds = append(menuIds, menuIdStr)
		}
	}

	menuIdsStr := strings.Join(menuIds, ",")

	if menuIdsStr != "" && userIdStr != "" {
		dataMenusAndTable, err := getDataMenuByIdsAndTable(menuIdsStr, tableIdStr)
		if err != nil {
			return nil, err
		}
		dataUsers, err := getUsersNameByIds(userIdStr)
		if err != nil {
			return nil, err
		}

		for idDataDetail, dataDetail := range res.Details {
			for _, menu := range dataMenusAndTable.Menus {
				if dataDetail.MenuId == menu.Id {
					res.Details[idDataDetail].MenuName = menu.Name
					res.Details[idDataDetail].Photo = menu.Photo
					res.Details[idDataDetail].Description = menu.Description
					break
				}
			}
		}
		if len(dataMenusAndTable.Tables) > 0 && res.TableId == dataMenusAndTable.Tables[0].Id {
			res.TableName = dataMenusAndTable.Tables[0].Name
		}
		if len(dataUsers) > 0 && res.UserId == dataUsers[0].UserId {
			res.OrderBy = dataUsers[0].FullName
		}
	}
	if res.TableName == "" {
		if strings.EqualFold(res.OrderType, "TAKEAWAY") {
			res.TableName = "Takeaway"
		} else if res.TableId == 0 {
			res.TableName = "No Table"
		}
	}

	return res, nil
}

func (s *transactionService) GetListTransactionsByUserId(request common.ParamsListRequest, userId int64, name string) (*response.Pagination[[]TransactionResponse], error) {
	res, err := s.repo.GetListTransactionsByUserId(request, userId)
	if err != nil {
		return nil, err
	}

	menuIds := []string{}
	tableIds := []string{}
	for _, data := range res.Data {
		tableIdStr := utils.Int64ToString(data.TableId)

		if data.TableId != 0 && !strings.Contains(strings.Join(tableIds, ","), tableIdStr) {
			tableIds = append(tableIds, tableIdStr)
		}

		for _, detail := range data.Details {
			menuIdStr := utils.IntToString(detail.MenuId)
			if detail.MenuId != 0 && !strings.Contains(strings.Join(menuIds, ","), menuIdStr) {
				menuIds = append(menuIds, menuIdStr)
			}
		}
	}

	menuIdsStr := strings.Join(menuIds, ",")
	tableIdsStr := strings.Join(tableIds, ",")
	if tableIdsStr == "" {
		tableIdsStr = "0"
	}
	if menuIdsStr != "" {
		dataMenusAndTable, err := getDataMenuByIdsAndTable(menuIdsStr, tableIdsStr)
		if err != nil {
			return nil, err
		}

		for i, data := range res.Data {
			for idDataDetail, dataDetail := range data.Details {
				for _, menu := range dataMenusAndTable.Menus {
					if dataDetail.MenuId == menu.Id {
						res.Data[i].Details[idDataDetail].MenuName = menu.Name
						res.Data[i].Details[idDataDetail].Photo = menu.Photo
						res.Data[i].Details[idDataDetail].Description = menu.Description
						break
					}
				}
			}
			for _, table := range dataMenusAndTable.Tables {
				if data.TableId == table.Id {
					res.Data[i].TableName = table.Name
					break
				}
			}
			if res.Data[i].TableName == "" {
				if strings.EqualFold(res.Data[i].OrderType, "TAKEAWAY") {
					res.Data[i].TableName = "Takeaway"
				} else if res.Data[i].TableId == 0 {
					res.Data[i].TableName = "No Table"
				}
			}

			res.Data[i].OrderBy = name
		}
	} else {
		for i := range res.Data {
			if res.Data[i].TableName == "" {
				if strings.EqualFold(res.Data[i].OrderType, "TAKEAWAY") {
					res.Data[i].TableName = "Takeaway"
				} else if res.Data[i].TableId == 0 {
					res.Data[i].TableName = "No Table"
				}
			}
			res.Data[i].OrderBy = name
		}
	}

	return res, nil
}

func (s *transactionService) GetOneTransactionByUserId(request *common.OneRequest, userId int64, name string) (*TransactionResponse, error) {
	res, err := s.repo.GetOneTransactionByUserId(request.Id, userId)
	if err != nil {
		return nil, err
	}

	menuIds := []string{}
	tableIdStr := utils.Int64ToString(res.TableId)
	if tableIdStr == "" || tableIdStr == "0" {
		tableIdStr = "0"
	}

	for _, detail := range res.Details {
		menuIdStr := utils.IntToString(detail.MenuId)
		if detail.MenuId != 0 && !strings.Contains(strings.Join(menuIds, ","), menuIdStr) {
			menuIds = append(menuIds, menuIdStr)
		}
	}

	menuIdsStr := strings.Join(menuIds, ",")

	if menuIdsStr != "" {
		dataMenusAndTable, err := getDataMenuByIdsAndTable(menuIdsStr, tableIdStr)
		if err != nil {
			return nil, err
		}

		for idDataDetail, dataDetail := range res.Details {
			for _, menu := range dataMenusAndTable.Menus {
				if dataDetail.MenuId == menu.Id {
					res.Details[idDataDetail].MenuName = menu.Name
					res.Details[idDataDetail].Photo = menu.Photo
					res.Details[idDataDetail].Description = menu.Description
					break
				}
			}
		}
		if len(dataMenusAndTable.Tables) > 0 && res.TableId == dataMenusAndTable.Tables[0].Id {
			res.TableName = dataMenusAndTable.Tables[0].Name
		}
	}
	if res.TableName == "" {
		if strings.EqualFold(res.OrderType, "TAKEAWAY") {
			res.TableName = "Takeaway"
		} else if res.TableId == 0 {
			res.TableName = "No Table"
		}
	}

	res.OrderBy = name

	return res, nil
}

func (s *transactionService) UpdateOrderStatus(tx *sqlx.Tx, request UpdateOrderStatusRequest) error {
	err := s.repo.UpdateOrderStatus(tx, request.Id, request.UpdatedBy)
	if err != nil {
		return err
	}

	// Trigger real-time status update notification asynchronously
	go func() {
		// Wait a small duration to ensure DB transaction is committed
		time.Sleep(50 * time.Millisecond)

		orderDetail, err := s.GetOneTransaction(&common.OneRequest{Id: request.Id})
		if err != nil {
			slog.Error("Failed to fetch full order details for SSE update status", "error", err)
			return
		}

		ch, err := lib.GetChannel()
		if err != nil {
			slog.Error("Failed to get rabbitmq channel for status update SSE", "error", err)
			return
		}
		defer ch.Close()

		type ssePayloadObj struct {
			Event string               `json:"event"`
			Data  *TransactionResponse `json:"data"`
		}
		payloadObj := ssePayloadObj{
			Event: "update_order_status",
			Data:  orderDetail,
		}
		ssePayload, err := json.Marshal(payloadObj)
		if err != nil {
			slog.Error("Failed to marshal SSE payload", "error", err)
			return
		}

		// Publish to order.created fanout exchange
		err = lib.SendMessage(ch, "", "", "order.created", lib.ExchangeFanout, amqp.Publishing{
			ContentType: "application/json",
			Body:        ssePayload,
		}, string(ssePayload), false, false, true, amqp.Table{})
		if err != nil {
			slog.Error("Failed to publish SSE status update message", "error", err)
		}
	}()

	return nil
}

func (s *transactionService) SetRatingMenu(tx *sqlx.Tx, request SetRatingMenuRequest) error {
	idMenu, err := s.repo.SetRatingMenu(tx, request.Id, request.Rating, request.UpdatedBy)
	if err != nil {
		return err
	}

	ch, err := lib.GetChannel()
	if err != nil {
		slog.Error("Failed to get RabbitMQ channel", "error", err)
		return nil
	}
	payload := []byte(fmt.Sprintf(`{"id": %d, "rating": %d, "updatedBy": %d}`, idMenu, request.Rating, request.UpdatedBy))
	err = lib.SendMessage(ch, "menu.set_rating", "menu.set_rating", "", lib.ExchangeDirect, amqp.Publishing{
		ContentType: "application/json",
		Body:        payload,
	}, string(payload), true, false, false, amqp.Table{})
	if err != nil {
		slog.Error("Failed to send rating message to RabbitMQ", "error", err)
		return nil
	}

	return nil
}

func (s *transactionService) SummaryReportTransactions(startDate string, endDate string) (*SummaryReportData, error) {
	return s.repo.SummaryReportTransactions(startDate, endDate)
}

func calculateTotalPriceMenu(menus []MenuResponse, request *CreateTransactionRequest) float64 {
	var total float64
	for _, menu := range menus {
		itemPrice := menu.Price
		if menu.EffectivePrice > 0 {
			itemPrice = menu.EffectivePrice
		}
		for iD, data := range request.Datas {
			if menu.Id == data.MenuID {
				request.Datas[iD].Price = itemPrice
				request.Datas[iD].Total = itemPrice * float64(data.Qty)
				total += itemPrice * float64(data.Qty)
			}
		}
	}

	return total
}

func createSignature(params string, body string, timestamp string) (string, error) {

	message := params + timestamp + body
	
	slog.Info("Generating HMAC Signature", 
		slog.String("payload", message),
		slog.String("params", params),
		slog.String("body", body),
	)

	signature, err := utils.GenerateHMAC(message)
	if err != nil {
		slog.Error("Failed to generate HMAC", slog.String("error", err.Error()))
		return "", response.InternalServerError("Internal Server Error", nil)
	}

	return signature, nil
}

func paymentUseWallet(userId int64, total float64, pin string) error {
	// Implement the logic to check the user's wallet balance
	urlWallet := fmt.Sprintf("%s/api/internal/pay", config.Config.ServiceWalletUrl)

	// Create the request body
	bodyRequest := PaymentRequest{
		UserId: userId,
		Amount: total,
		Pin:    pin,
	}

	timestamp := time.Now().UTC().Format(time.RFC3339)

	// Marshal ke JSON (sekali saja)
	bodyBytes, err := json.Marshal(bodyRequest)
	if err != nil {
		return err
	}

	// Simpan versi string-nya untuk signature
	bodyString := string(bodyBytes)

	signature, err := createSignature("", bodyString, timestamp)

	if err != nil {
		return err
	}

	_, err = utils.InternalRequest(signature, timestamp, urlWallet, "POST", bytes.NewReader(bodyBytes))

	if err != nil {
		return err
	}

	return nil
}

func getAvailableMenuByIdsAndTableById(ids string, tableId int64) ([]MenuResponse, error) {
	params := url.Values{}
	params.Add("ids", ids)
	params.Add("tableId", fmt.Sprintf("%d", tableId))
	
	queryString := params.Encode()
	urlMasterData := fmt.Sprintf("%s/api/internal/available-menus-table?%s", config.Config.ServiceMasterDataUrl, queryString)

	timestamp := time.Now().UTC().Format(time.RFC3339)

	signature, err := createSignature(queryString, "", timestamp)

	if err != nil {
		return nil, err
	}

	body, err := utils.InternalRequest(signature, timestamp, urlMasterData, "GET", nil)
	if err != nil {
		return nil, err
	}

	var menus InternalMenuResponse
	err = json.Unmarshal(body, &menus)
	if err != nil {
		slog.Error("Failed to unmarshal response body", "error", err)
		return nil, response.InternalServerError("Internal Server Error", nil)
	}

	return menus.Data, nil
}

func getDataMenuByIdsAndTable(ids string, tableIds string) (GetMenusAndTableResponse, error) {
	params := url.Values{}
	params.Add("ids", ids)
	params.Add("tableIds", tableIds)

	queryString := params.Encode()
	urlMasterData := fmt.Sprintf("%s/api/internal/data-menus-table?%s", config.Config.ServiceMasterDataUrl, queryString)

	timestamp := time.Now().UTC().Format(time.RFC3339)

	signature, err := createSignature(queryString, "", timestamp)

	if err != nil {
		return GetMenusAndTableResponse{}, err
	}

	body, err := utils.InternalRequest(signature, timestamp, urlMasterData, "GET", nil)
	if err != nil {
		return GetMenusAndTableResponse{}, err
	}

	var data InternalGetMenusAndTableResponse
	err = json.Unmarshal(body, &data)
	if err != nil {
		slog.Error("Failed to unmarshal response body", "error", err)
		return GetMenusAndTableResponse{}, response.InternalServerError("Internal Server Error", nil)
	}

	return data.Data, nil
}

func getUsersNameByIds(ids string) ([]UserResponse, error) {
	if ids == "" || ids == "0" {
		return []UserResponse{}, nil
	}
	params := url.Values{}
	params.Add("ids", ids)
	
	queryString := params.Encode()
	urlAccount := fmt.Sprintf("%s/api/internal/name-users?%s", config.Config.ServiceAccountUrl, queryString)

	timestamp := time.Now().UTC().Format(time.RFC3339)

	signature, err := createSignature(queryString, "", timestamp)

	if err != nil {
		return nil, err
	}

	body, err := utils.InternalRequest(signature, timestamp, urlAccount, "GET", nil)
	if err != nil {
		return nil, err
	}

	var data InternalGetUserResponse
	err = json.Unmarshal(body, &data)
	if err != nil {
		slog.Error("Failed to unmarshal response body", "error", err)
		return nil, response.InternalServerError("Internal Server Error", nil)
	}

	return data.Data, nil
}

func payPosWallet(paymentCode string, amount float64, orderId int64) (*PosWalletPayResponse, error) {
	urlWallet := fmt.Sprintf("%s/api/internal/pos/wallet/pay", config.Config.ServiceWalletUrl)

	reqPayload := PosWalletPayRequest{
		PaymentCode: paymentCode,
		Amount:      amount,
		OrderId:     orderId,
	}

	bodyBytes, err := json.Marshal(reqPayload)
	if err != nil {
		return nil, response.InternalServerError("Failed to marshal wallet pay payload", nil)
	}

	timestamp := time.Now().UTC().Format(time.RFC3339)
	signature, err := createSignature("", string(bodyBytes), timestamp)
	if err != nil {
		return nil, response.InternalServerError("Failed to create signature", nil)
	}

	body, err := utils.InternalRequest(signature, timestamp, urlWallet, "POST", bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, err
	}

	type InternalPosWalletResponse struct {
		Success bool                 `json:"success"`
		Message string               `json:"message"`
		Data    PosWalletPayResponse `json:"data"`
	}

	var res InternalPosWalletResponse
	err = json.Unmarshal(body, &res)
	if err != nil {
		slog.Error("Failed to unmarshal POS wallet response", "error", err)
		return nil, response.InternalServerError("Failed to parse wallet response", nil)
	}

	return &res.Data, nil
}

type PosWalletRefundRequest struct {
	UserId  int64   `json:"userId"`
	Amount  float64 `json:"amount"`
	OrderId int64   `json:"orderId"`
}

func refundPosWallet(userId int64, amount float64, orderId int64) error {
	urlWallet := fmt.Sprintf("%s/api/internal/pos/wallet/refund", config.Config.ServiceWalletUrl)

	reqPayload := PosWalletRefundRequest{
		UserId:  userId,
		Amount:  amount,
		OrderId: orderId,
	}

	bodyBytes, err := json.Marshal(reqPayload)
	if err != nil {
		return response.InternalServerError("Failed to marshal wallet refund payload", nil)
	}

	timestamp := time.Now().UTC().Format(time.RFC3339)
	signature, err := createSignature("", string(bodyBytes), timestamp)
	if err != nil {
		return response.InternalServerError("Failed to create signature", nil)
	}

	_, err = utils.InternalRequest(signature, timestamp, urlWallet, "POST", bytes.NewReader(bodyBytes))
	if err != nil {
		slog.Error("Failed to auto-refund POS wallet payment", "error", err, "userId", userId, "orderId", orderId)
		return err
	}
	slog.Info("Successfully auto-refunded POS wallet payment", "userId", userId, "orderId", orderId, "amount", amount)
	return nil
}

func chargePosQris(orderId string, amount float64, customerName string, customerEmail string) (*PosQrisChargeResponse, error) {
	urlWallet := fmt.Sprintf("%s/api/internal/pos/qris/charge", config.Config.ServiceWalletUrl)

	reqPayload := PosQrisChargeRequest{
		OrderId:       orderId,
		GrossAmount:   amount,
		CustomerName:  customerName,
		CustomerEmail: customerEmail,
	}

	bodyBytes, err := json.Marshal(reqPayload)
	if err != nil {
		return nil, response.InternalServerError("Failed to marshal QRIS charge payload", nil)
	}

	timestamp := time.Now().UTC().Format(time.RFC3339)
	signature, err := createSignature("", string(bodyBytes), timestamp)
	if err != nil {
		return nil, response.InternalServerError("Failed to create signature", nil)
	}

	body, err := utils.InternalRequest(signature, timestamp, urlWallet, "POST", bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, err
	}

	type InternalPosQrisResponse struct {
		Success bool                  `json:"success"`
		Message string                `json:"message"`
		Data    PosQrisChargeResponse `json:"data"`
	}

	var res InternalPosQrisResponse
	err = json.Unmarshal(body, &res)
	if err != nil {
		slog.Error("Failed to unmarshal POS QRIS response", "error", err)
		return nil, response.InternalServerError("Failed to parse QRIS response", nil)
	}

	return &res.Data, nil
}

func checkPosQrisStatus(orderId string) (map[string]interface{}, error) {
	urlWallet := fmt.Sprintf("%s/api/internal/pos/qris/status/%s", config.Config.ServiceWalletUrl, orderId)

	timestamp := time.Now().UTC().Format(time.RFC3339)
	signature, err := createSignature("", "", timestamp)
	if err != nil {
		return nil, response.InternalServerError("Failed to create signature", nil)
	}

	body, err := utils.InternalRequest(signature, timestamp, urlWallet, "GET", nil)
	if err != nil {
		return nil, err
	}

	var res map[string]interface{}
	err = json.Unmarshal(body, &res)
	if err != nil {
		return nil, err
	}
	return res, nil
}

