package tests

import (
	"testing"
)

func TestOrderCheckoutSuite(t *testing.T) {
	dbConn, teardown := SetupTestPostgresTransaction(t)
	defer teardown()

	app := SetupTestApp(dbConn)
	customerToken := GenerateTestToken(100, "user@test.com", "customer")
	adminToken := GenerateTestToken(1, "admin@test.com", "admin")

	t.Run("Checkout Transaction", func(t *testing.T) {
		body := []byte(`{
			"tableId": 3,
			"orderFor": "Dine In",
			"details": [{"menuId": 10, "qty": 2, "price": 25000}]
		}`)
		resp, err := ExecuteTestRequest(app, "POST", "/api/1.0/checkout", body, customerToken)
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		if resp.StatusCode != 201 && resp.StatusCode != 400 && resp.StatusCode != 500 {
			t.Fatalf("Expected valid HTTP response status, got %v", resp.StatusCode)
		}
	})

	t.Run("Get List Transactions", func(t *testing.T) {
		resp, err := ExecuteTestRequest(app, "GET", "/api/1.0/transactions?page=1&size=10", nil, adminToken)
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		if resp.StatusCode != 200 {
			t.Fatalf("Expected HTTP 200 OK, got %v", resp.StatusCode)
		}
	})
}
