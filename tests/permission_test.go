package tests

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"eka-dev.cloud/transaction-service/config"
	"eka-dev.cloud/transaction-service/lib"
	"eka-dev.cloud/transaction-service/modules/voucher"
	"eka-dev.cloud/transaction-service/utils/common"
	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
)

func generateCustomTxToken(userId int64, role string, roleId int, permissions map[string]common.PermissionAction) string {
	secret := config.Config.SecretJwt
	if secret == "" {
		secret = "super-secret-jwt-key"
		config.Config.SecretJwt = secret
	}

	if lib.RedisClient != nil && permissions != nil {
		permBytes, _ := json.Marshal(permissions)
		lib.RedisClient.Set(context.Background(), fmt.Sprintf("auth:role_permissions:%d", roleId), string(permBytes), 24*time.Hour)
	}

	claims := common.Claims{
		FullName: "Custom Tx User",
		Email:    "customtx@perm.test",
		UserId:   userId,
		Type:     "ACCESS",
		Role:     role,
		RoleId:   roleId,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, _ := token.SignedString([]byte(secret))
	return tokenString
}

func TestPBACPermissionSuite(t *testing.T) {
	dbConn, teardown := SetupTestPostgresTransaction(t)
	defer teardown()

	app := SetupTestApp(dbConn)

	t.Run("Super Admin voucher creation allowed", func(t *testing.T) {
		token := GenerateTestToken(1, "admin@coffe.com", "admin")

		isPub := true
		reqBody := voucher.CreateVoucherRequest{
			Code:          "ADMINPROMO10",
			DiscountType:  "FIXED",
			DiscountValue: 10000,
			Quota:         50,
			MinPurchase:   30000,
			ExpiredAt:     time.Now().Add(48 * time.Hour).Format(time.RFC3339),
			IsPublic:      &isPub,
		}
		bodyBytes, _ := json.Marshal(reqBody)

		resp, err := ExecuteTestRequest(app, "POST", "/api/1.0/vouchers", bodyBytes, token)
		assert.NoError(t, err)
		assert.Equal(t, http.StatusCreated, resp.StatusCode)
	})

	t.Run("Barista denied voucher creation when permission missing", func(t *testing.T) {
		perms := map[string]common.PermissionAction{
			"order":   {View: true, Edit: true},
			"voucher": {View: true, Create: false, Edit: false, Delete: false},
		}
		token := generateCustomTxToken(3, "barista_novoucher@coffe.com", 3, perms)

		isPub := true
		reqBody := voucher.CreateVoucherRequest{
			Code:          "BARISTANOVOUCHER",
			DiscountType:  "FIXED",
			DiscountValue: 10000,
			Quota:         50,
			MinPurchase:   30000,
			ExpiredAt:     time.Now().Add(48 * time.Hour).Format(time.RFC3339),
			IsPublic:      &isPub,
		}
		bodyBytes, _ := json.Marshal(reqBody)

		resp, err := ExecuteTestRequest(app, "POST", "/api/1.0/vouchers", bodyBytes, token)
		assert.NoError(t, err)
		assert.Equal(t, http.StatusForbidden, resp.StatusCode)
	})

	t.Run("Customer forbidden from sales summary report", func(t *testing.T) {
		token := GenerateTestToken(2, "customer@coffe.com", "user")

		resp, err := ExecuteTestRequest(app, "GET", "/api/1.0/transactions/summary-report", nil, token)
		assert.NoError(t, err)
		assert.Equal(t, http.StatusForbidden, resp.StatusCode)
	})

	t.Run("Returns 500 Internal Server Error when permission lookup fails", func(t *testing.T) {
		token := generateCustomTxToken(99, "unknown_role@coffe.com", 9999, nil)
		if lib.RedisClient != nil {
			lib.RedisClient.Del(context.Background(), "auth:role_permissions:9999")
		}

		resp, err := ExecuteTestRequest(app, "GET", "/api/1.0/transactions/summary-report", nil, token)
		assert.NoError(t, err)
		assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
	})
}
