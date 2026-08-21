package tests

import (
	"encoding/json"
	"io"
	"testing"
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
}
