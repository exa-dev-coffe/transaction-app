package tests

import (
	"encoding/json"
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

	t.Run("Validate Voucher - 10% Percentage Discount", func(t *testing.T) {
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
		_ = json.Unmarshal(respBody, &res)

		if !res.Data.Valid {
			t.Fatalf("Expected voucher DISCOUNT10 to be valid")
		}
		if res.Data.DiscountAmount != 10000 {
			t.Errorf("Expected discountAmount 10000, got %f", res.Data.DiscountAmount)
		}
		if res.Data.FinalTotal != 90000 {
			t.Errorf("Expected finalTotal 90000, got %f", res.Data.FinalTotal)
		}
	})

	t.Run("Validate Voucher - Fixed 20K Discount", func(t *testing.T) {
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
		_ = json.Unmarshal(respBody, &res)

		if !res.Data.Valid {
			t.Fatalf("Expected voucher HEMAT20K to be valid")
		}
		if res.Data.DiscountAmount != 20000 {
			t.Errorf("Expected discountAmount 20000, got %f", res.Data.DiscountAmount)
		}
		if res.Data.FinalTotal != 55000 {
			t.Errorf("Expected finalTotal 55000, got %f", res.Data.FinalTotal)
		}
	})

	t.Run("Validate Voucher - Min Purchase Not Met", func(t *testing.T) {
		body := []byte(`{"code":"MIN100K","orderTotal":50000}`)
		resp, err := ExecuteTestRequest(app, "POST", "/api/1.0/transactions/validate-voucher", body, customerToken)
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}

		respBody, _ := io.ReadAll(resp.Body)
		var res validateVoucherResponse
		_ = json.Unmarshal(respBody, &res)

		if res.Data.Valid {
			t.Errorf("Expected MIN100K to be invalid due to min purchase requirement")
		}
	})

	t.Run("Validate Voucher - Quota Reached", func(t *testing.T) {
		body := []byte(`{"code":"SOLD_OUT","orderTotal":50000}`)
		resp, err := ExecuteTestRequest(app, "POST", "/api/1.0/transactions/validate-voucher", body, customerToken)
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}

		respBody, _ := io.ReadAll(resp.Body)
		var res validateVoucherResponse
		_ = json.Unmarshal(respBody, &res)

		if res.Data.Valid {
			t.Errorf("Expected SOLD_OUT to be invalid due to 0 quota")
		}
	})

	t.Run("Validate Voucher - Already Used", func(t *testing.T) {
		body := []byte(`{"code":"ONCE_ONLY","orderTotal":50000}`)
		resp, err := ExecuteTestRequest(app, "POST", "/api/1.0/transactions/validate-voucher", body, customerToken)
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}

		respBody, _ := io.ReadAll(resp.Body)
		var res validateVoucherResponse
		_ = json.Unmarshal(respBody, &res)

		if res.Data.Valid {
			t.Errorf("Expected ONCE_ONLY to be invalid because user 100 already used it")
		}
	})

	t.Run("Get List Vouchers", func(t *testing.T) {
		resp, err := ExecuteTestRequest(app, "GET", "/api/1.0/vouchers", nil, adminToken)
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		if resp.StatusCode != 200 {
			t.Fatalf("Expected HTTP 200 OK, got %v", resp.StatusCode)
		}
	})
}
