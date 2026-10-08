package tests

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"testing"
	"time"

	"eka-dev.cloud/transaction-service/lib"
	"eka-dev.cloud/transaction-service/modules/transaction"
	"eka-dev.cloud/transaction-service/modules/voucher"
	amqp "github.com/rabbitmq/amqp091-go"
)

type createTransactionResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	Data    struct {
		Id int64 `json:"id"`
	} `json:"data"`
}

type orderDetailItem struct {
	Id         int     `json:"id"`
	MenuId     int     `json:"menuId"`
	Qty        int     `json:"qty"`
	Price      float64 `json:"price"`
	TotalPrice float64 `json:"totalPrice"`
}

type orderItem struct {
	Id         int64             `json:"id"`
	UserId     int64             `json:"userId"`
	TotalPrice float64           `json:"totalPrice"`
	OrderFor   string            `json:"orderFor"`
	Details    []orderDetailItem `json:"details"`
}

type getListTransactionsResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	Data    struct {
		Data        []orderItem `json:"data"`
		TotalData   int         `json:"totalData"`
		TotalPages  int         `json:"totalPages"`
		CurrentPage int         `json:"currentPage"`
		PageSize    int         `json:"pageSize"`
		LastPage    bool        `json:"lastPage"`
	} `json:"data"`
}

type getTransactionDetailResponse struct {
	Success bool      `json:"success"`
	Message string    `json:"message"`
	Data    orderItem `json:"data"`
}

type getSummaryReportResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	Data    struct {
		DailyData []struct {
			Total      float64 `json:"total"`
			TotalOrder int64   `json:"totalOrder"`
			CreatedAt  string  `json:"createdAt"`
		} `json:"dailyData"`
		StatusBreakdown []struct {
			Status int `json:"status"`
			Count  int `json:"count"`
		} `json:"statusBreakdown"`
	} `json:"data"`
}

type genericOrderResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

func TestOrderCheckoutSuite(t *testing.T) {
	dbConn, teardown := SetupTestPostgresTransaction(t)
	defer teardown()

	mockServer := SetupMockExternalServices()
	defer mockServer.Close()

	app := SetupTestApp(dbConn)
	customerToken := GenerateTestToken(100, "user@test.com", "customer")
	adminToken := GenerateTestToken(1, "admin@test.com", "admin")

	// Seed prerequisite transaction data with order_status = 1 (PENDING) into PostgreSQL
	_, err := dbConn.Exec(`
		INSERT INTO th_user_checkouts (id, user_id, order_status, total_price, order_for, table_id, created_by)
		VALUES (201, 100, 1, 50000.00, 'Dine In', 1, 100) ON CONFLICT (id) DO NOTHING;
		INSERT INTO td_user_checkouts (id, ref_id, menu_id, qty, price, total_price)
		VALUES (601, 201, 10, 2, 25000.00, 50000.00) ON CONFLICT (id) DO NOTHING;
	`)
	if err != nil {
		t.Fatalf("Failed to seed prerequisite test data: %v", err)
	}

	t.Run("POST /checkout - Create Order Transaction", func(t *testing.T) {
		body := []byte(`{
			"tableId": 3,
			"orderFor": "Dine In",
			"pin": "123456",
			"datas": [{"menuId": 10, "qty": 2, "price": 25000.00, "total": 50000.00}],
			"total": 50000.00
		}`)
		resp, err := ExecuteTestRequest(app, "POST", "/api/1.0/checkout", body, customerToken)
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		if resp.StatusCode != 201 {
			respBody, _ := io.ReadAll(resp.Body)
			t.Fatalf("Expected HTTP 201 Created, got %v: %s", resp.StatusCode, string(respBody))
		}

		respBody, _ := io.ReadAll(resp.Body)
		var res createTransactionResponse
		if err := json.Unmarshal(respBody, &res); err != nil {
			t.Fatalf("Failed to unmarshal response JSON: %v", err)
		}

		if !res.Success {
			t.Errorf("Expected success true, got false")
		}
		if res.Message == "" {
			t.Errorf("Expected non-empty creation response message")
		}

		// Verify transaction saved in PostgreSQL database
		var count int
		_ = dbConn.Get(&count, "SELECT count(*) FROM th_user_checkouts WHERE user_id = 100")
		if count < 1 {
			t.Errorf("Expected transaction to be created in PostgreSQL DB, found count: %d", count)
		}
	})

	t.Run("GET /transactions - Admin Get List Transactions", func(t *testing.T) {
		resp, err := ExecuteTestRequest(app, "GET", "/api/1.0/transactions?page=1&size=10", nil, adminToken)
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		if resp.StatusCode != 200 {
			respBody, _ := io.ReadAll(resp.Body)
			t.Fatalf("Expected HTTP 200 OK, got %v: %s", resp.StatusCode, string(respBody))
		}

		respBody, _ := io.ReadAll(resp.Body)
		var res getListTransactionsResponse
		if err := json.Unmarshal(respBody, &res); err != nil {
			t.Fatalf("Failed to unmarshal response JSON: %v", err)
		}

		if !res.Success {
			t.Errorf("Expected success true, got false")
		}
		if res.Message != "Success" {
			t.Errorf("Expected message 'Success', got '%s'", res.Message)
		}
		if res.Data.CurrentPage != 1 {
			t.Errorf("Expected currentPage 1, got %d", res.Data.CurrentPage)
		}
		if res.Data.PageSize != 10 {
			t.Errorf("Expected pageSize 10, got %d", res.Data.PageSize)
		}
		if res.Data.TotalData < 1 {
			t.Errorf("Expected totalData at least 1, got %d", res.Data.TotalData)
		}
		if len(res.Data.Data) == 0 {
			t.Fatalf("Expected transactions data array to be non-empty")
		}

		// Assert first transaction item details
		firstItem := res.Data.Data[0]
		if firstItem.Id <= 0 {
			t.Errorf("Expected valid ID for transaction index 0, got %d", firstItem.Id)
		}
		if firstItem.TotalPrice <= 0 {
			t.Errorf("Expected TotalPrice > 0 for index 0, got %f", firstItem.TotalPrice)
		}
	})

	t.Run("GET /transactions/detail - Admin Get Transaction Detail", func(t *testing.T) {
		resp, err := ExecuteTestRequest(app, "GET", "/api/1.0/transactions/detail?id=201", nil, adminToken)
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		if resp.StatusCode != 200 {
			respBody, _ := io.ReadAll(resp.Body)
			t.Fatalf("Expected HTTP 200 OK, got %v: %s", resp.StatusCode, string(respBody))
		}

		respBody, _ := io.ReadAll(resp.Body)
		var res getTransactionDetailResponse
		if err := json.Unmarshal(respBody, &res); err != nil {
			t.Fatalf("Failed to unmarshal response JSON: %v", err)
		}

		if !res.Success {
			t.Errorf("Expected success true, got false")
		}
		if res.Message != "Success" {
			t.Errorf("Expected message 'Success', got '%s'", res.Message)
		}
		if res.Data.Id != 201 {
			t.Errorf("Expected detail transaction ID 201, got %d", res.Data.Id)
		}
		if res.Data.TotalPrice != 50000 {
			t.Errorf("Expected TotalPrice 50000, got %f", res.Data.TotalPrice)
		}
		if len(res.Data.Details) == 0 {
			t.Fatalf("Expected order detail items to be non-empty")
		}
		if res.Data.Details[0].Id != 601 {
			t.Errorf("Expected order detail ID 601 for index 0, got %d", res.Data.Details[0].Id)
		}
	})

	t.Run("PATCH /transactions/update-order-status - Admin Update Order Status", func(t *testing.T) {
		body := []byte(`{"id": 201}`)
		resp, err := ExecuteTestRequest(app, "PATCH", "/api/1.0/transactions/update-order-status", body, adminToken)
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		if resp.StatusCode != 200 {
			respBody, _ := io.ReadAll(resp.Body)
			t.Fatalf("Expected HTTP 200 OK, got %v: %s", resp.StatusCode, string(respBody))
		}

		respBody, _ := io.ReadAll(resp.Body)
		var res genericOrderResponse
		if err := json.Unmarshal(respBody, &res); err != nil {
			t.Fatalf("Failed to unmarshal response JSON: %v", err)
		}

		if !res.Success {
			t.Errorf("Expected success true, got false")
		}
		if res.Message == "" {
			t.Errorf("Expected non-empty response message")
		}
	})

	t.Run("GET /transactions/summary-report - Admin Sales Summary Report", func(t *testing.T) {
		resp, err := ExecuteTestRequest(app, "GET", "/api/1.0/transactions/summary-report?startDate=2020-01-01&endDate=2030-12-31", nil, adminToken)
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		if resp.StatusCode != 200 {
			respBody, _ := io.ReadAll(resp.Body)
			t.Fatalf("Expected HTTP 200 OK, got %v: %s", resp.StatusCode, string(respBody))
		}

		respBody, _ := io.ReadAll(resp.Body)
		var res getSummaryReportResponse
		if err := json.Unmarshal(respBody, &res); err != nil {
			t.Fatalf("Failed to unmarshal response JSON: %v", err)
		}

		if !res.Success {
			t.Errorf("Expected success true, got false")
		}
		if res.Message != "Success" {
			t.Errorf("Expected message 'Success', got '%s'", res.Message)
		}
		if res.Data.DailyData == nil {
			t.Errorf("Expected dailyData in summary report data to be non-nil")
		}
		if res.Data.StatusBreakdown == nil {
			t.Errorf("Expected statusBreakdown in summary report data to be non-nil")
		}
	})

	t.Run("POST /checkout - Create Order Transaction With Valid Voucher Code", func(t *testing.T) {
		// 1. Seed a valid voucher in PostgreSQL
		_, _ = dbConn.Exec(`DELETE FROM tm_vouchers WHERE code = 'CHECKOUT_PROMO'`)

		var voucherId int64
		err := dbConn.QueryRow(`
			INSERT INTO tm_vouchers (code, discount_type, discount_value, min_purchase, quota, is_active, expired_at)
			VALUES ('CHECKOUT_PROMO', 'FIXED', 10000.00, 20000.00, 10, true, CURRENT_TIMESTAMP + INTERVAL '1 day')
			RETURNING id
		`).Scan(&voucherId)
		if err != nil {
			t.Fatalf("Failed to seed voucher for test: %v", err)
		}

		body := []byte(`{
			"tableId": 3,
			"orderFor": "Dine In Voucher",
			"pin": "123456",
			"voucherCode": "CHECKOUT_PROMO",
			"datas": [{"menuId": 10, "qty": 2, "price": 25000.00, "total": 50000.00}],
			"total": 50000.00
		}`)
		resp, err := ExecuteTestRequest(app, "POST", "/api/1.0/checkout", body, customerToken)
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		if resp.StatusCode != 201 {
			respBody, _ := io.ReadAll(resp.Body)
			t.Fatalf("Expected HTTP 201 Created, got %v: %s", resp.StatusCode, string(respBody))
		}

		respBody, _ := io.ReadAll(resp.Body)
		var res createTransactionResponse
		if err := json.Unmarshal(respBody, &res); err != nil {
			t.Fatalf("Failed to unmarshal response JSON: %v", err)
		}

		if !res.Success {
			t.Errorf("Expected success true, got false: %s", string(respBody))
		}

		// Verify transaction record in th_user_checkouts has voucher_id, discount_amount, and net total_price
		var createdId int64
		var dbVoucherId int64
		var discountAmount float64
		var totalPrice float64
		err = dbConn.QueryRow(`SELECT id, voucher_id, discount_amount, total_price FROM th_user_checkouts WHERE user_id = 100 AND order_for = 'Dine In Voucher' ORDER BY id DESC LIMIT 1`).Scan(&createdId, &dbVoucherId, &discountAmount, &totalPrice)
		if err != nil {
			t.Fatalf("Failed to query created transaction from DB: %v", err)
		}

		if dbVoucherId != voucherId {
			t.Errorf("Expected dbVoucherId %d, got %d", voucherId, dbVoucherId)
		}
		if discountAmount != 10000.00 {
			t.Errorf("Expected discountAmount 10000.00, got %f", discountAmount)
		}
		if totalPrice != 40000.00 {
			t.Errorf("Expected net totalPrice 40000.00 (50000 - 10000), got %f", totalPrice)
		}

		// Verify voucher usage logged in tr_voucher_usages
		var usageCount int
		_ = dbConn.Get(&usageCount, `SELECT count(*) FROM tr_voucher_usages WHERE checkout_id = $1 AND voucher_id = $2`, createdId, voucherId)
		if usageCount != 1 {
			t.Errorf("Expected 1 usage record in tr_voucher_usages, got %d", usageCount)
		}

		// Verify voucher quota decremented from 10 to 9
		var currentQuota int
		_ = dbConn.Get(&currentQuota, `SELECT quota FROM tm_vouchers WHERE id = $1`, voucherId)
		if currentQuota != 9 {
			t.Errorf("Expected quota decremented to 9, got %d", currentQuota)
		}
	})

	t.Run("POST /checkout - Reject Checkout With Expired Voucher Code", func(t *testing.T) {
		// Seed an expired voucher
		_, _ = dbConn.Exec(`DELETE FROM tm_vouchers WHERE code = 'EXPIRED_CHECKOUT_PROMO'`)
		_, err := dbConn.Exec(`
			INSERT INTO tm_vouchers (code, discount_type, discount_value, min_purchase, quota, is_active, expired_at)
			VALUES ('EXPIRED_CHECKOUT_PROMO', 'FIXED', 10000.00, 10000.00, 10, true, CURRENT_TIMESTAMP - INTERVAL '1 day')
		`)
		if err != nil {
			t.Fatalf("Failed to seed expired voucher: %v", err)
		}

		body := []byte(`{
			"tableId": 3,
			"orderFor": "Dine In Expired",
			"pin": "123456",
			"voucherCode": "EXPIRED_CHECKOUT_PROMO",
			"datas": [{"menuId": 10, "qty": 2, "price": 25000.00, "total": 50000.00}],
			"total": 50000.00
		}`)
		resp, err := ExecuteTestRequest(app, "POST", "/api/1.0/checkout", body, customerToken)
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		if resp.StatusCode != 400 && resp.StatusCode != 404 {
			t.Fatalf("Expected HTTP 400/404 for expired voucher, got %v", resp.StatusCode)
		}
	})

	t.Run("POST /checkout - Create Order Transaction With Product Promo Discount", func(t *testing.T) {
		body := []byte(`{
			"tableId": 1,
			"orderFor": "Dine In Product Promo",
			"pin": "123456",
			"datas": [{"menuId": 11, "qty": 2, "price": 24000.00, "total": 48000.00}],
			"total": 48000.00
		}`)
		resp, err := ExecuteTestRequest(app, "POST", "/api/1.0/checkout", body, customerToken)
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		if resp.StatusCode != 201 {
			respBody, _ := io.ReadAll(resp.Body)
			t.Fatalf("Expected HTTP 201 Created, got %v: %s", resp.StatusCode, string(respBody))
		}

		// Verify transaction record created in th_user_checkouts with effective price total (48,000)
		var createdId int64
		var totalPrice float64
		err = dbConn.QueryRow(`SELECT id, total_price FROM th_user_checkouts WHERE user_id = 100 AND order_for = 'Dine In Product Promo' ORDER BY id DESC LIMIT 1`).Scan(&createdId, &totalPrice)
		if err != nil {
			t.Fatalf("Failed to query product promo transaction from DB: %v", err)
		}

		if totalPrice != 48000.00 {
			t.Errorf("Expected effective total_price 48000.00, got %f", totalPrice)
		}

		// Verify promotion usage logged in tr_promotion_usages (promotion_id = 99, menu_id = 11, qty = 2, discount_amount = 12000)
		var promoUsageCount int
		var promoDiscountSum float64
		err = dbConn.QueryRow(`SELECT count(*), COALESCE(SUM(discount_amount), 0) FROM tr_promotion_usages WHERE transaction_id = $1 AND promotion_id = 99`, createdId).Scan(&promoUsageCount, &promoDiscountSum)
		if err != nil {
			t.Fatalf("Failed to query tr_promotion_usages from DB: %v", err)
		}

		if promoUsageCount != 1 {
			t.Errorf("Expected 1 promotion usage record in tr_promotion_usages, got %d", promoUsageCount)
		}
		if promoDiscountSum != 12000.00 {
			t.Errorf("Expected total promo discount sum 12000.00 (6000 * 2), got %f", promoDiscountSum)
		}
	})

	t.Run("POST /checkout - Create Order Transaction With BOTH Product Promo Discount AND Voucher Code", func(t *testing.T) {
		// Seed a voucher for this combined test
		_, _ = dbConn.Exec(`DELETE FROM tm_vouchers WHERE code = 'COMBINED_PROMO'`)
		var voucherId int64
		err := dbConn.QueryRow(`
			INSERT INTO tm_vouchers (code, discount_type, discount_value, min_purchase, quota, is_active, expired_at)
			VALUES ('COMBINED_PROMO', 'FIXED', 8000.00, 20000.00, 5, true, CURRENT_TIMESTAMP + INTERVAL '1 day')
			RETURNING id
		`).Scan(&voucherId)
		if err != nil {
			t.Fatalf("Failed to seed combined promo voucher: %v", err)
		}

		// Menu 11 has effective price 24,000 * 2 = 48,000. Plus voucher COMBINED_PROMO discount 8,000. Net total = 40,000.
		body := []byte(`{
			"tableId": 1,
			"orderFor": "Dine In Combined Promo",
			"pin": "123456",
			"voucherCode": "COMBINED_PROMO",
			"datas": [{"menuId": 11, "qty": 2, "price": 24000.00, "total": 48000.00}],
			"total": 48000.00
		}`)
		resp, err := ExecuteTestRequest(app, "POST", "/api/1.0/checkout", body, customerToken)
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		if resp.StatusCode != 201 {
			respBody, _ := io.ReadAll(resp.Body)
			t.Fatalf("Expected HTTP 201 Created, got %v: %s", resp.StatusCode, string(respBody))
		}

		// Verify transaction record in th_user_checkouts has net total 40,000 (48,000 - 8,000)
		var createdId int64
		var dbVoucherId int64
		var discountAmount float64
		var totalPrice float64
		err = dbConn.QueryRow(`SELECT id, voucher_id, discount_amount, total_price FROM th_user_checkouts WHERE user_id = 100 AND order_for = 'Dine In Combined Promo' ORDER BY id DESC LIMIT 1`).Scan(&createdId, &dbVoucherId, &discountAmount, &totalPrice)
		if err != nil {
			t.Fatalf("Failed to query combined promo transaction from DB: %v", err)
		}

		if dbVoucherId != voucherId {
			t.Errorf("Expected dbVoucherId %d, got %d", voucherId, dbVoucherId)
		}
		if discountAmount != 8000.00 {
			t.Errorf("Expected voucher discountAmount 8000.00, got %f", discountAmount)
		}
		if totalPrice != 40000.00 {
			t.Errorf("Expected net totalPrice 40000.00 (48000 - 8000), got %f", totalPrice)
		}

		// Verify product promo usage logged in tr_promotion_usages (promotion_id = 99, sum = 12,000)
		var promoUsageCount int
		var promoDiscountSum float64
		_ = dbConn.QueryRow(`SELECT count(*), COALESCE(SUM(discount_amount), 0) FROM tr_promotion_usages WHERE transaction_id = $1 AND promotion_id = 99`, createdId).Scan(&promoUsageCount, &promoDiscountSum)
		if promoUsageCount != 1 || promoDiscountSum != 12000.00 {
			t.Errorf("Expected 1 promo usage with sum 12000.00, got count %d sum %f", promoUsageCount, promoDiscountSum)
		}

		// Verify voucher usage logged in tr_voucher_usages (voucher_id = voucherId, discount_amount = 8,000)
		var voucherUsageCount int
		_ = dbConn.Get(&voucherUsageCount, `SELECT count(*) FROM tr_voucher_usages WHERE checkout_id = $1 AND voucher_id = $2`, createdId, voucherId)
		if voucherUsageCount != 1 {
			t.Errorf("Expected 1 voucher usage record in tr_voucher_usages, got %d", voucherUsageCount)
		}
	})
}

func TestPosOrderCheckoutSuite(t *testing.T) {
	dbConn, teardown := SetupTestPostgresTransaction(t)
	defer teardown()

	mockServer := SetupMockExternalServices()
	defer mockServer.Close()

	app := SetupTestApp(dbConn)
	adminToken := GenerateTestToken(1, "admin@test.com", "admin")
	customerToken := GenerateTestToken(100, "user@test.com", "customer")

	t.Run("POST /pos/checkout - CASH Dine-In Order", func(t *testing.T) {
		body := []byte(`{
			"orderType": "DINE_IN",
			"tableId": 1,
			"orderFor": "Walk-in Budi",
			"paymentMethod": "CASH",
			"cashAmount": 100000.00,
			"cashChange": 50000.00,
			"datas": [{"menuId": 10, "qty": 2, "price": 25000.00, "total": 50000.00}]
		}`)
		resp, err := ExecuteTestRequest(app, "POST", "/api/1.0/pos/checkout", body, adminToken)
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		if resp.StatusCode != 201 {
			respBody, _ := io.ReadAll(resp.Body)
			t.Fatalf("Expected HTTP 201 Created, got %v: %s", resp.StatusCode, string(respBody))
		}

		var res struct {
			Success bool `json:"success"`
			Data    struct {
				Id            int64   `json:"id"`
				OrderType     string  `json:"orderType"`
				PaymentMethod string  `json:"paymentMethod"`
				PaymentStatus string  `json:"paymentStatus"`
				CashAmount    float64 `json:"cashAmount"`
				CashChange    float64 `json:"cashChange"`
				IsCashier     bool    `json:"isCashier"`
			} `json:"data"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&res)

		if !res.Success || res.Data.Id == 0 {
			t.Errorf("Expected valid POS checkout response, got %+v", res)
		}
		if res.Data.PaymentStatus != "PAID" {
			t.Errorf("Expected paymentStatus 'PAID', got '%s'", res.Data.PaymentStatus)
		}
		if res.Data.PaymentMethod != "CASH" {
			t.Errorf("Expected paymentMethod 'CASH', got '%s'", res.Data.PaymentMethod)
		}
		if !res.Data.IsCashier {
			t.Errorf("Expected isCashier true, got false")
		}

		// Check database record
		var isCashier bool
		var paymentStatus string
		var cashChange float64
		err = dbConn.QueryRow(`SELECT is_cashier, payment_status, cash_change FROM th_user_checkouts WHERE id = $1`, res.Data.Id).Scan(&isCashier, &paymentStatus, &cashChange)
		if err != nil {
			t.Fatalf("Failed to query th_user_checkouts: %v", err)
		}
		if !isCashier || paymentStatus != "PAID" || cashChange != 50000.00 {
			t.Errorf("DB values mismatch: isCashier=%v, paymentStatus=%s, cashChange=%f", isCashier, paymentStatus, cashChange)
		}
	})

	t.Run("POST /pos/checkout - CASH Takeaway Order (No Table)", func(t *testing.T) {
		body := []byte(`{
			"orderType": "TAKEAWAY",
			"orderFor": "Takeaway Guest",
			"paymentMethod": "CASH",
			"cashAmount": 50000.00,
			"cashChange": 0.00,
			"datas": [{"menuId": 10, "qty": 2, "price": 25000.00, "total": 50000.00}]
		}`)
		resp, err := ExecuteTestRequest(app, "POST", "/api/1.0/pos/checkout", body, adminToken)
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		if resp.StatusCode != 201 {
			respBody, _ := io.ReadAll(resp.Body)
			t.Fatalf("Expected HTTP 201 Created, got %v: %s", resp.StatusCode, string(respBody))
		}

		var res struct {
			Success bool `json:"success"`
			Data    struct {
				Id        int64  `json:"id"`
				OrderType string `json:"orderType"`
				TableId   *int64 `json:"tableId"`
			} `json:"data"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&res)
		if res.Data.OrderType != "TAKEAWAY" {
			t.Errorf("Expected orderType 'TAKEAWAY', got '%s'", res.Data.OrderType)
		}
	})

	t.Run("POST /pos/checkout - WALLET Payment with Dynamic Code", func(t *testing.T) {
		body := []byte(`{
			"orderType": "DINE_IN",
			"tableId": 1,
			"orderFor": "Wallet Customer",
			"paymentMethod": "WALLET",
			"walletPaymentCode": "839201",
			"datas": [{"menuId": 10, "qty": 2, "price": 25000.00, "total": 50000.00}]
		}`)
		resp, err := ExecuteTestRequest(app, "POST", "/api/1.0/pos/checkout", body, adminToken)
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		if resp.StatusCode != 201 {
			respBody, _ := io.ReadAll(resp.Body)
			t.Fatalf("Expected HTTP 201 Created, got %v: %s", resp.StatusCode, string(respBody))
		}

		var res struct {
			Success bool `json:"success"`
			Data    struct {
				Id            int64  `json:"id"`
				PaymentMethod string `json:"paymentMethod"`
				PaymentStatus string `json:"paymentStatus"`
			} `json:"data"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&res)

		if res.Data.PaymentMethod != "WALLET" || res.Data.PaymentStatus != "PAID" {
			t.Errorf("Expected WALLET PAID, got %+v", res.Data)
		}

		// Verify user_id resolved from wallet-service is saved in DB
		var dbUserId int64
		err = dbConn.QueryRow(`SELECT user_id FROM th_user_checkouts WHERE id = $1`, res.Data.Id).Scan(&dbUserId)
		if err != nil {
			t.Fatalf("Failed to query user_id from DB: %v", err)
		}
		if dbUserId != 100 {
			t.Errorf("Expected dbUserId 100 resolved from wallet-service, got %d", dbUserId)
		}
	})

	t.Run("POST /pos/checkout - MIDTRANS Dynamic QRIS Charge & Sync", func(t *testing.T) {
		body := []byte(`{
			"orderType": "DINE_IN",
			"tableId": 1,
			"orderFor": "QRIS Customer",
			"paymentMethod": "MIDTRANS",
			"datas": [{"menuId": 10, "qty": 2, "price": 25000.00, "total": 50000.00}]
		}`)
		resp, err := ExecuteTestRequest(app, "POST", "/api/1.0/pos/checkout", body, adminToken)
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		if resp.StatusCode != 201 {
			respBody, _ := io.ReadAll(resp.Body)
			t.Fatalf("Expected HTTP 201 Created, got %v: %s", resp.StatusCode, string(respBody))
		}

		var res struct {
			Success bool `json:"success"`
			Data    struct {
				Id            int64  `json:"id"`
				PaymentMethod string `json:"paymentMethod"`
				PaymentStatus string `json:"paymentStatus"`
				QrString      string `json:"qrString"`
				QrUrl         string `json:"qrUrl"`
			} `json:"data"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&res)

		if res.Data.PaymentMethod != "MIDTRANS" || res.Data.PaymentStatus != "PENDING" {
			t.Errorf("Expected MIDTRANS PENDING, got %+v", res.Data)
		}
		if res.Data.QrString == "" || res.Data.QrUrl == "" {
			t.Errorf("Expected qrString and qrUrl populated, got qrString='%s', qrUrl='%s'", res.Data.QrString, res.Data.QrUrl)
		}

		// Now test Syncing Midtrans QRIS status
		syncResp, err := ExecuteTestRequest(app, "POST", fmt.Sprintf("/api/1.0/pos/transactions/%d/sync-midtrans", res.Data.Id), nil, adminToken)
		if err != nil {
			t.Fatalf("Sync request failed: %v", err)
		}
		if syncResp.StatusCode != 200 {
			syncBody, _ := io.ReadAll(syncResp.Body)
			t.Fatalf("Expected HTTP 200 OK on sync, got %v: %s", syncResp.StatusCode, string(syncBody))
		}

		// Verify status changed to PAID in DB
		var currentPaymentStatus string
		_ = dbConn.QueryRow(`SELECT payment_status FROM th_user_checkouts WHERE id = $1`, res.Data.Id).Scan(&currentPaymentStatus)
		if currentPaymentStatus != "PAID" {
			t.Errorf("Expected payment_status to be updated to 'PAID', got '%s'", currentPaymentStatus)
		}
	})

	t.Run("SETTLEMENT - Event-driven SettlePosQrisPayment", func(t *testing.T) {
		// 1. Create a pending POS QRIS transaction directly in DB
		var txId int64
		err := dbConn.QueryRow(`
			INSERT INTO th_user_checkouts (user_id, table_id, order_for, order_type, payment_method, payment_status, total_price, is_cashier)
			VALUES (1, 1, 'Async Customer', 'DINE_IN', 'MIDTRANS', 'PENDING', 75000.00, true)
			RETURNING id
		`).Scan(&txId)
		if err != nil {
			t.Fatalf("Failed to insert pending checkout: %v", err)
		}

		_, err = dbConn.Exec(`
			INSERT INTO td_user_checkouts (ref_id, menu_id, qty, price, total_price, created_by)
			VALUES ($1, 10, 3, 25000.00, 75000.00, 1)
		`, txId)
		if err != nil {
			t.Fatalf("Failed to insert checkout item: %v", err)
		}

		// 2. Call SettlePosQrisPayment with POS order reference
		voucherRepo := voucher.NewVoucherRepository(dbConn)
		voucherService := voucher.NewVoucherService(voucherRepo, dbConn)
		txRepo := transaction.NewTransactionRepository(dbConn)
		txService := transaction.NewTransactionService(txRepo, voucherService, dbConn)

		orderRef := fmt.Sprintf("POS-%d-1724410293000", txId)
		settledRes, err := txService.SettlePosQrisPayment(context.Background(), orderRef, "PAID")
		if err != nil {
			t.Fatalf("SettlePosQrisPayment failed: %v", err)
		}
		if settledRes.PaymentStatus != "PAID" {
			t.Errorf("Expected settled status PAID, got %s", settledRes.PaymentStatus)
		}

		// Verify database state is updated to PAID
		var updatedStatus string
		_ = dbConn.QueryRow(`SELECT payment_status FROM th_user_checkouts WHERE id = $1`, txId).Scan(&updatedStatus)
		if updatedStatus != "PAID" {
			t.Errorf("Expected DB payment_status 'PAID', got '%s'", updatedStatus)
		}
	})

	t.Run("RABBITMQ LISTENER - Real End-to-End pos.payment.settled Consumer", func(t *testing.T) {
		ch, err := lib.GetChannel()
		if err != nil {
			t.Skipf("Skipping RabbitMQ listener test because channel unavailable: %v", err)
			return
		}
		defer ch.Close()

		voucherRepo := voucher.NewVoucherRepository(dbConn)
		voucherService := voucher.NewVoucherService(voucherRepo, dbConn)
		txRepo := transaction.NewTransactionRepository(dbConn)
		txService := transaction.NewTransactionService(txRepo, voucherService, dbConn)

		// Start real RabbitMQ consumer listener
		transaction.NewListener(ch, txService, dbConn)

		// 1. Insert PENDING POS transaction in DB
		var txId int64
		err = dbConn.QueryRow(`
			INSERT INTO th_user_checkouts (user_id, table_id, order_for, order_type, payment_method, payment_status, total_price, is_cashier)
			VALUES (1, 1, 'RabbitMQ Consumer Test', 'DINE_IN', 'MIDTRANS', 'PENDING', 50000.00, true)
			RETURNING id
		`).Scan(&txId)
		if err != nil {
			t.Fatalf("Failed to insert pending checkout: %v", err)
		}

		_, err = dbConn.Exec(`
			INSERT INTO td_user_checkouts (ref_id, menu_id, qty, price, total_price, created_by)
			VALUES ($1, 10, 2, 25000.00, 50000.00, 1)
		`, txId)
		if err != nil {
			t.Fatalf("Failed to insert checkout item: %v", err)
		}

		// 2. Publish real message to RabbitMQ exchange "pos.payment.settled" (Direct exchange)
		orderRef := fmt.Sprintf("POS-%d-1724410293000", txId)
		eventBody := fmt.Sprintf(`{"orderRef":"%s","paymentStatus":"PAID","grossAmount":50000.0}`, orderRef)

		err = lib.SendMessage(ch, "", "", "pos.payment.settled", lib.ExchangeDirect, amqp.Publishing{
			ContentType: "application/json",
			Body:        []byte(eventBody),
		}, eventBody, true, false, false, amqp.Table{})
		if err != nil {
			t.Fatalf("Failed to publish RabbitMQ message: %v", err)
		}

		// 3. Poll DB until listener receives message and updates payment_status to PAID
		var currentStatus string
		for i := 0; i < 30; i++ {
			time.Sleep(100 * time.Millisecond)
			_ = dbConn.QueryRow(`SELECT payment_status FROM th_user_checkouts WHERE id = $1`, txId).Scan(&currentStatus)
			if currentStatus == "PAID" {
				break
			}
		}

		if currentStatus != "PAID" {
			t.Errorf("Expected payment_status to be updated to 'PAID' by RabbitMQ listener, got '%s'", currentStatus)
		}
	})

	t.Run("POST /pos/checkout - Forbidden for Normal Customer Token", func(t *testing.T) {
		body := []byte(`{
			"orderType": "DINE_IN",
			"tableId": 1,
			"orderFor": "Hacker Guest",
			"paymentMethod": "CASH",
			"cashAmount": 50000.00,
			"datas": [{"menuId": 10, "qty": 2, "price": 25000.00, "total": 50000.00}]
		}`)
		resp, err := ExecuteTestRequest(app, "POST", "/api/1.0/pos/checkout", body, customerToken)
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		if resp.StatusCode != 403 {
			t.Errorf("Expected HTTP 403 Forbidden for customer without pos permission, got %v", resp.StatusCode)
		}
	})

	t.Run("PATCH /pos/transactions/:id/change-payment - Forbidden for Normal Customer Token", func(t *testing.T) {
		body := []byte(`{
			"paymentMethod": "CASH",
			"cashAmount": 50000.00,
			"cashChange": 0.0
		}`)
		resp, err := ExecuteTestRequest(app, "PATCH", "/api/1.0/pos/transactions/201/change-payment", body, customerToken)
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		if resp.StatusCode != 403 {
			t.Errorf("Expected HTTP 403 Forbidden for customer without pos permission, got %v", resp.StatusCode)
		}
	})

	t.Run("PATCH /pos/transactions/:id/change-payment - Successful Switch to CASH", func(t *testing.T) {
		var txId int64
		err := dbConn.QueryRow(`
			INSERT INTO th_user_checkouts (user_id, table_id, order_for, order_type, payment_method, payment_status, total_price, is_cashier)
			VALUES (1, 1, 'Pending QRIS Customer', 'DINE_IN', 'MIDTRANS', 'PENDING', 50000.00, true)
			RETURNING id
		`).Scan(&txId)
		if err != nil {
			t.Fatalf("Failed to insert pending checkout for test: %v", err)
		}

		_, err = dbConn.Exec(`
			INSERT INTO td_user_checkouts (ref_id, menu_id, qty, price, total_price, created_by)
			VALUES ($1, 10, 2, 25000.00, 50000.00, 1)
		`, txId)
		if err != nil {
			t.Fatalf("Failed to insert checkout item: %v", err)
		}

		body := []byte(`{
			"paymentMethod": "CASH",
			"cashAmount": 100000.00,
			"cashChange": 50000.00
		}`)
		url := fmt.Sprintf("/api/1.0/pos/transactions/%d/change-payment", txId)
		resp, err := ExecuteTestRequest(app, "PATCH", url, body, adminToken)
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		if resp.StatusCode != 200 {
			respBody, _ := io.ReadAll(resp.Body)
			t.Fatalf("Expected HTTP 200 OK, got %v with body %s", resp.StatusCode, string(respBody))
		}

		var updatedMethod, updatedStatus string
		var updatedCash, updatedChange float64
		_ = dbConn.QueryRow(`
			SELECT payment_method, payment_status, cash_amount, cash_change 
			FROM th_user_checkouts WHERE id = $1
		`, txId).Scan(&updatedMethod, &updatedStatus, &updatedCash, &updatedChange)

		if updatedMethod != "CASH" {
			t.Errorf("Expected DB payment_method 'CASH', got '%s'", updatedMethod)
		}
		if updatedStatus != "PAID" {
			t.Errorf("Expected DB payment_status 'PAID', got '%s'", updatedStatus)
		}
		if updatedCash != 100000.00 {
			t.Errorf("Expected cash_amount 100000.00, got %v", updatedCash)
		}
		if updatedChange != 50000.00 {
			t.Errorf("Expected cash_change 50000.00, got %v", updatedChange)
		}
	})

	t.Run("PATCH /pos/transactions/:id/change-payment - Validation Errors (Insufficient Cash)", func(t *testing.T) {
		var txId int64
		err := dbConn.QueryRow(`
			INSERT INTO th_user_checkouts (user_id, table_id, order_for, order_type, payment_method, payment_status, total_price, is_cashier)
			VALUES (1, 1, 'Insufficient Cash Test', 'DINE_IN', 'MIDTRANS', 'PENDING', 50000.00, true)
			RETURNING id
		`).Scan(&txId)
		if err != nil {
			t.Fatalf("Failed to insert pending checkout: %v", err)
		}

		_, err = dbConn.Exec(`
			INSERT INTO td_user_checkouts (ref_id, menu_id, qty, price, total_price, created_by)
			VALUES ($1, 10, 2, 25000.00, 50000.00, 1)
		`, txId)
		if err != nil {
			t.Fatalf("Failed to insert checkout item: %v", err)
		}

		body := []byte(`{
			"paymentMethod": "CASH",
			"cashAmount": 20000.00,
			"cashChange": 0.0
		}`)
		url := fmt.Sprintf("/api/1.0/pos/transactions/%d/change-payment", txId)
		resp, err := ExecuteTestRequest(app, "PATCH", url, body, adminToken)
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		if resp.StatusCode != 400 {
			t.Errorf("Expected HTTP 400 Bad Request for insufficient cash, got %v", resp.StatusCode)
		}
	})

	t.Run("PATCH /pos/transactions/:id/change-payment - Successful Switch to WALLET", func(t *testing.T) {
		var txId int64
		err := dbConn.QueryRow(`
			INSERT INTO th_user_checkouts (user_id, table_id, order_for, order_type, payment_method, payment_status, total_price, is_cashier)
			VALUES (1, 1, 'Walk-in Guest', 'DINE_IN', 'MIDTRANS', 'PENDING', 50000.00, true)
			RETURNING id
		`).Scan(&txId)
		if err != nil {
			t.Fatalf("Failed to insert pending checkout for wallet test: %v", err)
		}

		_, err = dbConn.Exec(`
			INSERT INTO td_user_checkouts (ref_id, menu_id, qty, price, total_price, created_by)
			VALUES ($1, 10, 2, 25000.00, 50000.00, 1)
		`, txId)
		if err != nil {
			t.Fatalf("Failed to insert checkout item: %v", err)
		}

		body := []byte(`{
			"paymentMethod": "WALLET",
			"walletPaymentCode": "123456"
		}`)
		url := fmt.Sprintf("/api/1.0/pos/transactions/%d/change-payment", txId)
		resp, err := ExecuteTestRequest(app, "PATCH", url, body, adminToken)
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		if resp.StatusCode != 200 {
			respBody, _ := io.ReadAll(resp.Body)
			t.Fatalf("Expected HTTP 200 OK, got %v with body %s", resp.StatusCode, string(respBody))
		}

		var updatedMethod, updatedStatus, updatedOrderFor string
		var updatedUserId int64
		_ = dbConn.QueryRow(`
			SELECT payment_method, payment_status, user_id, order_for 
			FROM th_user_checkouts WHERE id = $1
		`, txId).Scan(&updatedMethod, &updatedStatus, &updatedUserId, &updatedOrderFor)

		if updatedMethod != "WALLET" {
			t.Errorf("Expected DB payment_method 'WALLET', got '%s'", updatedMethod)
		}
		if updatedStatus != "PAID" {
			t.Errorf("Expected DB payment_status 'PAID', got '%s'", updatedStatus)
		}
		if updatedUserId != 100 {
			t.Errorf("Expected DB user_id 100, got %v", updatedUserId)
		}
		if updatedOrderFor != "Test User" {
			t.Errorf("Expected DB order_for 'Test User', got '%s'", updatedOrderFor)
		}
	})

	t.Run("PATCH /pos/transactions/:id/change-payment - Disallowed Payment Method (MIDTRANS)", func(t *testing.T) {
		body := []byte(`{
			"paymentMethod": "MIDTRANS"
		}`)
		resp, err := ExecuteTestRequest(app, "PATCH", "/api/1.0/pos/transactions/1/change-payment", body, adminToken)
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		if resp.StatusCode != 400 {
			t.Errorf("Expected HTTP 400 Bad Request when switching to MIDTRANS, got %v", resp.StatusCode)
		}
	})
}
