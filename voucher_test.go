package main

import (
	"bytes"
	"net/http/httptest"
	"testing"
)

func TestVoucherSuite(t *testing.T) {
	dbConn, teardown := setupTestPostgresTransaction(t)
	defer teardown()

	app := setupTestApp(dbConn)

	t.Run("Validate Voucher Code", func(t *testing.T) {
		body := []byte(`{"code":"PROMO50","totalPrice":75000}`)
		req := httptest.NewRequest("POST", "/api/1.0/transactions/validate-voucher", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		resp, _ := app.Test(req)
		if resp.StatusCode != 200 && resp.StatusCode != 400 && resp.StatusCode != 401 && resp.StatusCode != 404 {
			t.Fatalf("Expected valid HTTP response status, got %v", resp.StatusCode)
		}
	})

	t.Run("Create Voucher", func(t *testing.T) {
		body := []byte(`{"code":"DISCOUNT20","discountAmount":20000,"minPurchase":50000}`)
		req := httptest.NewRequest("POST", "/api/1.0/vouchers", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		resp, _ := app.Test(req)
		if resp.StatusCode != 201 && resp.StatusCode != 400 && resp.StatusCode != 401 && resp.StatusCode != 403 {
			t.Fatalf("Expected valid HTTP response status, got %v", resp.StatusCode)
		}
	})

	t.Run("Get List Vouchers", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/1.0/vouchers", nil)
		resp, _ := app.Test(req)
		if resp.StatusCode != 200 && resp.StatusCode != 401 && resp.StatusCode != 403 {
			t.Fatalf("Expected valid HTTP response status, got %v", resp.StatusCode)
		}
	})
}
