package tests

import (
	"io"
	"testing"
)

func TestTransactionHistorySuite(t *testing.T) {
	dbConn, teardown := SetupTestPostgresTransaction(t)
	defer teardown()

	mockServer := SetupMockExternalServices()
	defer mockServer.Close()

	app := SetupTestApp(dbConn)
	customerToken := GenerateTestToken(100, "user@test.com", "customer")

	// Seed prerequisite transaction data with order_status = 2 (COMPLETED) for user ID 100
	_, err := dbConn.Exec(`
		INSERT INTO th_user_checkouts (id, user_id, order_status, total_price, order_for, table_id, created_by)
		VALUES (101, 100, 2, 50000.00, 'Dine In', 1, 100) ON CONFLICT (id) DO NOTHING;
		INSERT INTO td_user_checkouts (id, ref_id, menu_id, qty, price, total_price)
		VALUES (501, 101, 10, 2, 25000.00, 50000.00) ON CONFLICT (id) DO NOTHING;
	`)
	if err != nil {
		t.Fatalf("Failed to seed transaction history test data: %v", err)
	}

	t.Run("GET /history-checkouts - Customer History Checkouts List", func(t *testing.T) {
		resp, err := ExecuteTestRequest(app, "GET", "/api/1.0/history-checkouts?page=1&size=10", nil, customerToken)
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		if resp.StatusCode != 200 {
			respBody, _ := io.ReadAll(resp.Body)
			t.Fatalf("Expected HTTP 200 OK, got %v: %s", resp.StatusCode, string(respBody))
		}
	})

	t.Run("GET /history-checkouts/detail - Customer Checkout Detail", func(t *testing.T) {
		resp, err := ExecuteTestRequest(app, "GET", "/api/1.0/history-checkouts/detail?id=101", nil, customerToken)
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		if resp.StatusCode != 200 {
			respBody, _ := io.ReadAll(resp.Body)
			t.Fatalf("Expected HTTP 200 OK, got %v: %s", resp.StatusCode, string(respBody))
		}
	})

	t.Run("PATCH /history-checkouts/set-rating-menu - Customer Set Rating Menu", func(t *testing.T) {
		body := []byte(`{"id": 501, "rating": 5}`)
		resp, err := ExecuteTestRequest(app, "PATCH", "/api/1.0/history-checkouts/set-rating-menu", body, customerToken)
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		if resp.StatusCode != 200 {
			respBody, _ := io.ReadAll(resp.Body)
			t.Fatalf("Expected HTTP 200 OK, got %v: %s", resp.StatusCode, string(respBody))
		}

		// Verify rating updated in PostgreSQL DB
		var rating *int
		_ = dbConn.Get(&rating, "SELECT rating FROM td_user_checkouts WHERE id = 501")
		if rating == nil || *rating != 5 {
			t.Errorf("Expected rating 5 in PostgreSQL DB, got %v", rating)
		}
	})
}
