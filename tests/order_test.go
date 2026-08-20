package tests

import (
	"io"
	"testing"
)

type createTransactionResponse struct {
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

	// Seed prerequisite transaction data into PostgreSQL Test Database
	_, err := dbConn.Exec(`
		INSERT INTO th_user_checkouts (id, user_id, order_status, total_price, order_for, table_id, created_by)
		VALUES (101, 100, 1, 50000.00, 'Dine In', 1, 100) ON CONFLICT (id) DO NOTHING;
		INSERT INTO td_user_checkouts (id, ref_id, menu_id, qty, price, total_price)
		VALUES (501, 101, 10, 2, 25000.00, 50000.00) ON CONFLICT (id) DO NOTHING;
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
	})

	t.Run("GET /transactions/detail - Admin Get Transaction Detail", func(t *testing.T) {
		resp, err := ExecuteTestRequest(app, "GET", "/api/1.0/transactions/detail?id=101", nil, adminToken)
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		if resp.StatusCode != 200 {
			respBody, _ := io.ReadAll(resp.Body)
			t.Fatalf("Expected HTTP 200 OK, got %v: %s", resp.StatusCode, string(respBody))
		}
	})

	t.Run("PATCH /transactions/update-order-status - Admin Update Order Status", func(t *testing.T) {
		body := []byte(`{"id": 101}`)
		resp, err := ExecuteTestRequest(app, "PATCH", "/api/1.0/transactions/update-order-status", body, adminToken)
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		if resp.StatusCode != 200 {
			respBody, _ := io.ReadAll(resp.Body)
			t.Fatalf("Expected HTTP 200 OK, got %v: %s", resp.StatusCode, string(respBody))
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
	})
}
