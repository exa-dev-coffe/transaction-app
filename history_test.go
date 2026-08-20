package main

import (
	"net/http/httptest"
	"testing"
)

func TestTransactionHistorySuite(t *testing.T) {
	dbConn, teardown := setupTestPostgresTransaction(t)
	defer teardown()

	app := setupTestApp(dbConn)

	t.Run("Get User History Checkouts", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/1.0/history-checkouts", nil)
		resp, _ := app.Test(req)
		if resp.StatusCode != 200 && resp.StatusCode != 401 && resp.StatusCode != 403 {
			t.Fatalf("Expected valid HTTP response status, got %v", resp.StatusCode)
		}
	})
}
