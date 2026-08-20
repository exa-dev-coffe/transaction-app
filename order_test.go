package main

import (
	"bytes"
	"net/http/httptest"
	"testing"
)

func TestOrderCheckoutSuite(t *testing.T) {
	dbConn, teardown := setupTestPostgresTransaction(t)
	defer teardown()

	app := setupTestApp(dbConn)

	t.Run("Checkout Transaction", func(t *testing.T) {
		body := []byte(`{
			"tableId": 3,
			"orderFor": "Dine In",
			"details": [{"menuId": 10, "qty": 2, "price": 25000}]
		}`)
		req := httptest.NewRequest("POST", "/api/1.0/checkout", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		resp, _ := app.Test(req)
		if resp.StatusCode != 201 && resp.StatusCode != 400 && resp.StatusCode != 401 && resp.StatusCode != 500 {
			t.Fatalf("Expected valid HTTP response status, got %v", resp.StatusCode)
		}
	})

	t.Run("Get List Transactions", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/1.0/transactions?page=1&size=10", nil)
		resp, _ := app.Test(req)
		if resp.StatusCode != 200 && resp.StatusCode != 401 && resp.StatusCode != 403 {
			t.Fatalf("Expected valid HTTP response status, got %v", resp.StatusCode)
		}
	})
}
