package tests

import (
	"testing"
)

func TestTransactionHistorySuite(t *testing.T) {
	dbConn, teardown := SetupTestPostgresTransaction(t)
	defer teardown()

	app := SetupTestApp(dbConn)
	token := GenerateTestToken(100, "user@test.com", "customer")

	t.Run("Get User History Checkouts", func(t *testing.T) {
		resp, err := ExecuteTestRequest(app, "GET", "/api/1.0/history-checkouts", nil, token)
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		if resp.StatusCode != 200 {
			t.Fatalf("Expected HTTP 200 OK, got %v", resp.StatusCode)
		}
	})
}
