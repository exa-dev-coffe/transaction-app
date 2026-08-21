package tests

import (
	"encoding/json"
	"io"
	"testing"
)

type transactionDetailItem struct {
	Id         int     `json:"id"`
	MenuId     int     `json:"menuId"`
	Qty        int     `json:"qty"`
	Price      float64 `json:"price"`
	TotalPrice float64 `json:"totalPrice"`
	Rating     *int8   `json:"rating"`
}

type transactionItem struct {
	Id         int64                   `json:"id"`
	UserId     int64                   `json:"userId"`
	TotalPrice float64                 `json:"totalPrice"`
	OrderFor   string                  `json:"orderFor"`
	Details    []transactionDetailItem `json:"details"`
}

type getListHistoryCheckoutsResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	Data    struct {
		Data        []transactionItem `json:"data"`
		TotalData   int               `json:"totalData"`
		TotalPages  int               `json:"totalPages"`
		CurrentPage int               `json:"currentPage"`
		PageSize    int               `json:"pageSize"`
		LastPage    bool              `json:"lastPage"`
	} `json:"data"`
}

type getHistoryCheckoutDetailResponse struct {
	Success bool            `json:"success"`
	Message string          `json:"message"`
	Data    transactionItem `json:"data"`
}

type genericHistoryResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

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

		respBody, _ := io.ReadAll(resp.Body)
		var res getListHistoryCheckoutsResponse
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
			t.Fatalf("Expected history checkouts data array to be non-empty")
		}

		// Assert first item details
		firstItem := res.Data.Data[0]
		if firstItem.Id != 101 {
			t.Errorf("Expected transaction ID 101 for index 0, got %d", firstItem.Id)
		}
		if firstItem.TotalPrice != 50000 {
			t.Errorf("Expected TotalPrice 50000 for index 0, got %f", firstItem.TotalPrice)
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

		respBody, _ := io.ReadAll(resp.Body)
		var res getHistoryCheckoutDetailResponse
		if err := json.Unmarshal(respBody, &res); err != nil {
			t.Fatalf("Failed to unmarshal response JSON: %v", err)
		}

		if !res.Success {
			t.Errorf("Expected success true, got false")
		}
		if res.Message != "Success" {
			t.Errorf("Expected message 'Success', got '%s'", res.Message)
		}
		if res.Data.Id != 101 {
			t.Errorf("Expected detail transaction ID 101, got %d", res.Data.Id)
		}
		if res.Data.TotalPrice != 50000 {
			t.Errorf("Expected TotalPrice 50000, got %f", res.Data.TotalPrice)
		}
		if len(res.Data.Details) == 0 {
			t.Fatalf("Expected detail menu items to be non-empty")
		}
		if res.Data.Details[0].Id != 501 {
			t.Errorf("Expected menu detail ID 501 for index 0, got %d", res.Data.Details[0].Id)
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

		respBody, _ := io.ReadAll(resp.Body)
		var res genericHistoryResponse
		if err := json.Unmarshal(respBody, &res); err != nil {
			t.Fatalf("Failed to unmarshal response JSON: %v", err)
		}

		if !res.Success {
			t.Errorf("Expected success true, got false")
		}
		if res.Message == "" {
			t.Errorf("Expected non-empty response message")
		}

		// Verify rating updated in PostgreSQL DB
		var rating *int
		_ = dbConn.Get(&rating, "SELECT rating FROM td_user_checkouts WHERE id = 501")
		if rating == nil || *rating != 5 {
			t.Errorf("Expected rating 5 in PostgreSQL DB, got %v", rating)
		}
	})
}
