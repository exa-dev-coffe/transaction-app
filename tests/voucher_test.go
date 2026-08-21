package tests

import (
	"encoding/json"
	"fmt"
	"io"
	"testing"
)

type validateVoucherResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	Data    struct {
		Valid          bool    `json:"valid"`
		DiscountAmount float64 `json:"discountAmount"`
		FinalTotal     float64 `json:"finalTotal"`
		Message        string  `json:"message"`
	} `json:"data"`
}

type createVoucherResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	Data    struct {
		ID int64 `json:"id"`
	} `json:"data"`
}

type voucherItem struct {
	ID            int64   `json:"id"`
	Code          string  `json:"code"`
	DiscountType  string  `json:"discountType"`
	DiscountValue float64 `json:"discountValue"`
	MaxDiscount   float64 `json:"maxDiscount"`
	MinPurchase   float64 `json:"minPurchase"`
	Quota         int     `json:"quota"`
	IsActive      bool    `json:"isActive"`
}

type getListVouchersResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	Data    struct {
		Data        []voucherItem `json:"data"`
		TotalData   int           `json:"totalData"`
		TotalPages  int           `json:"totalPages"`
		CurrentPage int           `json:"currentPage"`
		PageSize    int           `json:"pageSize"`
		LastPage    bool          `json:"lastPage"`
	} `json:"data"`
}

type genericResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

func TestVoucherSuite(t *testing.T) {
	dbConn, teardown := SetupTestPostgresTransaction(t)
	defer teardown()

	app := SetupTestApp(dbConn)
	customerToken := GenerateTestToken(100, "user@test.com", "customer")
	adminToken := GenerateTestToken(1, "admin@test.com", "admin")

	// Seed Real Vouchers into PostgreSQL Test Database
	_, err := dbConn.Exec(`
		INSERT INTO tm_vouchers (id, code, discount_type, discount_value, max_discount, min_purchase, quota, is_active, expired_at)
		VALUES 
			(10, 'DISCOUNT10', 'PERCENTAGE', 10.00, 15000.00, 50000.00, 10, true, NOW() + INTERVAL '1 day'),
			(11, 'HEMAT20K', 'FIXED', 20000.00, 0.00, 50000.00, 5, true, NOW() + INTERVAL '1 day'),
			(12, 'MIN100K', 'FIXED', 10000.00, 0.00, 100000.00, 5, true, NOW() + INTERVAL '1 day'),
			(13, 'SOLD_OUT', 'FIXED', 10000.00, 0.00, 10000.00, 0, true, NOW() + INTERVAL '1 day'),
			(14, 'ONCE_ONLY', 'FIXED', 10000.00, 0.00, 10000.00, 10, true, NOW() + INTERVAL '1 day')
		ON CONFLICT (id) DO NOTHING;

		INSERT INTO tr_voucher_usages (user_id, voucher_id, checkout_id, discount_amount)
		VALUES (100, 14, 999, 10000.00)
		ON CONFLICT DO NOTHING;
	`)
	if err != nil {
		t.Fatalf("Failed to seed real test vouchers into PostgreSQL: %v", err)
	}

	t.Run("POST /transactions/validate-voucher - 10% Percentage Discount", func(t *testing.T) {
		body := []byte(`{"code":"DISCOUNT10","orderTotal":100000}`)
		resp, err := ExecuteTestRequest(app, "POST", "/api/1.0/transactions/validate-voucher", body, customerToken)
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		if resp.StatusCode != 200 {
			t.Fatalf("Expected HTTP 200 OK, got %v", resp.StatusCode)
		}

		respBody, _ := io.ReadAll(resp.Body)
		var res validateVoucherResponse
		if err := json.Unmarshal(respBody, &res); err != nil {
			t.Fatalf("Failed to parse response JSON: %v", err)
		}

		if !res.Success {
			t.Errorf("Expected success to be true, got false")
		}
		if res.Message != "Success" {
			t.Errorf("Expected message 'Success', got '%s'", res.Message)
		}
		if !res.Data.Valid {
			t.Fatalf("Expected voucher DISCOUNT10 to be valid")
		}
		if res.Data.DiscountAmount != 10000 {
			t.Errorf("Expected discountAmount 10000, got %f", res.Data.DiscountAmount)
		}
		if res.Data.FinalTotal != 90000 {
			t.Errorf("Expected finalTotal 90000, got %f", res.Data.FinalTotal)
		}
		if res.Data.Message != "Voucher applied successfully" {
			t.Errorf("Expected data message 'Voucher applied successfully', got '%s'", res.Data.Message)
		}
	})

	t.Run("POST /transactions/validate-voucher - Fixed 20K Discount", func(t *testing.T) {
		body := []byte(`{"code":"HEMAT20K","orderTotal":75000}`)
		resp, err := ExecuteTestRequest(app, "POST", "/api/1.0/transactions/validate-voucher", body, customerToken)
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		if resp.StatusCode != 200 {
			t.Fatalf("Expected HTTP 200 OK, got %v", resp.StatusCode)
		}

		respBody, _ := io.ReadAll(resp.Body)
		var res validateVoucherResponse
		if err := json.Unmarshal(respBody, &res); err != nil {
			t.Fatalf("Failed to parse response JSON: %v", err)
		}

		if !res.Success {
			t.Errorf("Expected success to be true, got false")
		}
		if res.Message != "Success" {
			t.Errorf("Expected message 'Success', got '%s'", res.Message)
		}
		if !res.Data.Valid {
			t.Fatalf("Expected voucher HEMAT20K to be valid")
		}
		if res.Data.DiscountAmount != 20000 {
			t.Errorf("Expected discountAmount 20000, got %f", res.Data.DiscountAmount)
		}
		if res.Data.FinalTotal != 55000 {
			t.Errorf("Expected finalTotal 55000, got %f", res.Data.FinalTotal)
		}
		if res.Data.Message != "Voucher applied successfully" {
			t.Errorf("Expected data message 'Voucher applied successfully', got '%s'", res.Data.Message)
		}
	})

	t.Run("POST /transactions/validate-voucher - Min Purchase Not Met", func(t *testing.T) {
		body := []byte(`{"code":"MIN100K","orderTotal":50000}`)
		resp, err := ExecuteTestRequest(app, "POST", "/api/1.0/transactions/validate-voucher", body, customerToken)
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		if resp.StatusCode != 200 {
			t.Fatalf("Expected HTTP 200 OK, got %v", resp.StatusCode)
		}

		respBody, _ := io.ReadAll(resp.Body)
		var res validateVoucherResponse
		if err := json.Unmarshal(respBody, &res); err != nil {
			t.Fatalf("Failed to parse response JSON: %v", err)
		}

		if !res.Success {
			t.Errorf("Expected success to be true, got false")
		}
		if res.Data.Valid {
			t.Errorf("Expected MIN100K to be invalid due to min purchase requirement")
		}
		if res.Data.Message == "" {
			t.Errorf("Expected data.message to describe why voucher is invalid, got empty string")
		}
	})

	t.Run("POST /transactions/validate-voucher - Quota Reached", func(t *testing.T) {
		body := []byte(`{"code":"SOLD_OUT","orderTotal":50000}`)
		resp, err := ExecuteTestRequest(app, "POST", "/api/1.0/transactions/validate-voucher", body, customerToken)
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		if resp.StatusCode != 200 {
			t.Fatalf("Expected HTTP 200 OK, got %v", resp.StatusCode)
		}

		respBody, _ := io.ReadAll(resp.Body)
		var res validateVoucherResponse
		if err := json.Unmarshal(respBody, &res); err != nil {
			t.Fatalf("Failed to parse response JSON: %v", err)
		}

		if !res.Success {
			t.Errorf("Expected success to be true, got false")
		}
		if res.Data.Valid {
			t.Errorf("Expected SOLD_OUT to be invalid due to 0 quota")
		}
		if res.Data.Message != "Voucher quota has been reached" {
			t.Errorf("Expected message 'Voucher quota has been reached', got '%s'", res.Data.Message)
		}
	})

	t.Run("POST /transactions/validate-voucher - Already Used", func(t *testing.T) {
		body := []byte(`{"code":"ONCE_ONLY","orderTotal":50000}`)
		resp, err := ExecuteTestRequest(app, "POST", "/api/1.0/transactions/validate-voucher", body, customerToken)
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		if resp.StatusCode != 200 {
			t.Fatalf("Expected HTTP 200 OK, got %v", resp.StatusCode)
		}

		respBody, _ := io.ReadAll(resp.Body)
		var res validateVoucherResponse
		if err := json.Unmarshal(respBody, &res); err != nil {
			t.Fatalf("Failed to parse response JSON: %v", err)
		}

		if !res.Success {
			t.Errorf("Expected success to be true, got false")
		}
		if res.Data.Valid {
			t.Errorf("Expected ONCE_ONLY to be invalid because user 100 already used it")
		}
		if res.Data.Message != "You have already used this voucher" {
			t.Errorf("Expected message 'You have already used this voucher', got '%s'", res.Data.Message)
		}
	})

	t.Run("POST /vouchers - Admin Create Voucher", func(t *testing.T) {
		body := []byte(`{
			"code": "NEWPROMO30",
			"discountType": "PERCENTAGE",
			"discountValue": 30.0,
			"maxDiscount": 20000.0,
			"minPurchase": 50000.0,
			"quota": 20,
			"expiredAt": "2030-12-31 23:59:59"
		}`)
		resp, err := ExecuteTestRequest(app, "POST", "/api/1.0/vouchers", body, adminToken)
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		if resp.StatusCode != 201 {
			t.Fatalf("Expected HTTP 201 Created, got %v", resp.StatusCode)
		}

		respBody, _ := io.ReadAll(resp.Body)
		var res createVoucherResponse
		if err := json.Unmarshal(respBody, &res); err != nil {
			t.Fatalf("Failed to parse response JSON: %v", err)
		}

		if !res.Success {
			t.Errorf("Expected success to be true, got false")
		}
		if res.Message != "Voucher created successfully" {
			t.Errorf("Expected message 'Voucher created successfully', got '%s'", res.Message)
		}
		if res.Data.ID <= 0 {
			t.Errorf("Expected valid voucher ID in data, got %d", res.Data.ID)
		}

		// Verify record created in PostgreSQL DB across ALL inserted columns
		var (
			code          string
			discountType  string
			discountValue float64
			maxDiscount   float64
			minPurchase   float64
			quota         int
			isActive      bool
		)
		err = dbConn.QueryRow(`
			SELECT code, discount_type, discount_value, max_discount, min_purchase, quota, is_active 
			FROM tm_vouchers WHERE id = $1
		`, res.Data.ID).Scan(&code, &discountType, &discountValue, &maxDiscount, &minPurchase, &quota, &isActive)
		if err != nil {
			t.Fatalf("Failed to fetch created voucher ID %d from DB: %v", res.Data.ID, err)
		}
		if code != "NEWPROMO30" {
			t.Errorf("Expected code 'NEWPROMO30', got '%s'", code)
		}
		if discountType != "PERCENTAGE" {
			t.Errorf("Expected discount_type 'PERCENTAGE', got '%s'", discountType)
		}
		if discountValue != 30.0 {
			t.Errorf("Expected discount_value 30.0, got %f", discountValue)
		}
		if maxDiscount != 20000.0 {
			t.Errorf("Expected max_discount 20000.0, got %f", maxDiscount)
		}
		if minPurchase != 50000.0 {
			t.Errorf("Expected min_purchase 50000.0, got %f", minPurchase)
		}
		if quota != 20 {
			t.Errorf("Expected quota 20, got %d", quota)
		}
		if !isActive {
			t.Errorf("Expected is_active true, got false")
		}
	})

	t.Run("GET /vouchers - Admin Get List Vouchers", func(t *testing.T) {
		resp, err := ExecuteTestRequest(app, "GET", "/api/1.0/vouchers?page=1&size=10", nil, adminToken)
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		if resp.StatusCode != 200 {
			t.Fatalf("Expected HTTP 200 OK, got %v", resp.StatusCode)
		}

		respBody, _ := io.ReadAll(resp.Body)
		var res getListVouchersResponse
		if err := json.Unmarshal(respBody, &res); err != nil {
			t.Fatalf("Failed to parse response JSON: %v", err)
		}

		if !res.Success {
			t.Errorf("Expected success to be true, got false")
		}
		if res.Message != "Success" {
			t.Errorf("Expected message 'Success', got '%s'", res.Message)
		}

		// Assert pagination schema details
		if res.Data.CurrentPage != 1 {
			t.Errorf("Expected currentPage 1, got %d", res.Data.CurrentPage)
		}
		if res.Data.PageSize != 10 {
			t.Errorf("Expected pageSize 10, got %d", res.Data.PageSize)
		}
		if res.Data.TotalData < 5 {
			t.Errorf("Expected totalData at least 5, got %d", res.Data.TotalData)
		}

		// Assert array content details
		if len(res.Data.Data) == 0 {
			t.Fatalf("Expected voucher list in data.data to be non-empty")
		}

		// Assert first element schema & data correctness
		firstVoucher := res.Data.Data[0]
		if firstVoucher.ID <= 0 {
			t.Errorf("Expected valid ID for first voucher, got %d", firstVoucher.ID)
		}
		if firstVoucher.Code == "" {
			t.Errorf("Expected non-empty code for first voucher")
		}
		if firstVoucher.DiscountType != "PERCENTAGE" && firstVoucher.DiscountType != "FIXED" {
			t.Errorf("Expected discountType to be PERCENTAGE or FIXED, got '%s'", firstVoucher.DiscountType)
		}
	})

	t.Run("PATCH /vouchers/:id/status - Admin Update Voucher Status", func(t *testing.T) {
		body := []byte(`{"isActive": false}`)
		url := fmt.Sprintf("/api/1.0/vouchers/10/status")
		resp, err := ExecuteTestRequest(app, "PATCH", url, body, adminToken)
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		if resp.StatusCode != 200 {
			t.Fatalf("Expected HTTP 200 OK, got %v", resp.StatusCode)
		}

		respBody, _ := io.ReadAll(resp.Body)
		var res genericResponse
		if err := json.Unmarshal(respBody, &res); err != nil {
			t.Fatalf("Failed to parse response JSON: %v", err)
		}

		if !res.Success {
			t.Errorf("Expected success to be true, got false")
		}
		if res.Message != "Voucher status updated successfully" {
			t.Errorf("Expected message 'Voucher status updated successfully', got '%s'", res.Message)
		}

		var isActive bool
		_ = dbConn.Get(&isActive, "SELECT is_active FROM tm_vouchers WHERE id = 10")
		if isActive != false {
			t.Errorf("Expected voucher 10 is_active to be false in PostgreSQL DB")
		}
	})

	t.Run("DELETE /vouchers/:id - Admin Delete Voucher", func(t *testing.T) {
		url := fmt.Sprintf("/api/1.0/vouchers/11")
		resp, err := ExecuteTestRequest(app, "DELETE", url, nil, adminToken)
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		if resp.StatusCode != 200 {
			t.Fatalf("Expected HTTP 200 OK, got %v", resp.StatusCode)
		}

		respBody, _ := io.ReadAll(resp.Body)
		var res genericResponse
		if err := json.Unmarshal(respBody, &res); err != nil {
			t.Fatalf("Failed to parse response JSON: %v", err)
		}

		if !res.Success {
			t.Errorf("Expected success to be true, got false")
		}
		if res.Message != "Voucher deleted successfully" {
			t.Errorf("Expected message 'Voucher deleted successfully', got '%s'", res.Message)
		}

		var deletedAt *string
		_ = dbConn.Get(&deletedAt, "SELECT deleted_at FROM tm_vouchers WHERE id = 11")
		if deletedAt == nil {
			t.Errorf("Expected voucher 11 deleted_at to be populated in PostgreSQL DB")
		}
	})
}
