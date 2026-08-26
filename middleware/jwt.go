package middleware

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"eka-dev.cloud/transaction-service/config"
	"eka-dev.cloud/transaction-service/lib"
	"eka-dev.cloud/transaction-service/utils"
	"eka-dev.cloud/transaction-service/utils/common"
	"eka-dev.cloud/transaction-service/utils/response"
	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v5"
)

func getJwtKey() []byte {
	if config.Config.SecretJwt != "" {
		return []byte(config.Config.SecretJwt)
	}
	return []byte("super-secret-jwt-key")
}

func getTokenFromHeader(c *fiber.Ctx) string {
	bearer := c.Get("Authorization")
	if bearer == "" {
		return ""
	}
	if strings.HasPrefix(bearer, "Bearer ") {
		return bearer[len("Bearer "):]
	}
	return bearer
}

func validateToken(c *fiber.Ctx) (*common.Claims, error) {
	tokenString := getTokenFromHeader(c)
	if tokenString == "" {
		return nil, response.Unauthorized("Missing Token", nil)
	}

	claims := &common.Claims{}
	token, err := jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, response.Unauthorized("Unexpected signing method", nil)
		}
		return getJwtKey(), nil
	})

	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return nil, response.Unauthorized("Token expired", nil)
		}
		return nil, response.Unauthorized("Invalid token", nil)
	}

	if !token.Valid {
		return nil, response.Unauthorized("Invalid token", nil)
	}

	if strings.ToLower(claims.Type) != "access" {
		return nil, response.Unauthorized("Invalid token type", nil)
	}

	return claims, nil
}

func RequireAuth(c *fiber.Ctx) error {
	claims, err := validateToken(c)
	if err != nil {
		var appErr *response.AppError
		if errors.As(err, &appErr) {
			return err
		}
		return response.Unauthorized("Unauthorized", nil)
	}
	c.Locals("user", claims)
	return c.Next()
}

func RequireRole(roles ...string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		claims, err := validateToken(c)
		if err != nil {
			var appErr *response.AppError
			if errors.As(err, &appErr) {
				return err
			}
			return response.Unauthorized("Unauthorized", nil)
		}

		userRole := claims.Role
		for _, role := range roles {
			if strings.EqualFold(userRole, role) {
				c.Locals("user", claims)
				return c.Next()
			}
		}
		return response.Forbidden("Forbidden", nil)
	}
}

// FetchRolePermissions retrieves role permissions from Redis or falls back to Account-Service. Returns error if both fail.
func FetchRolePermissions(roleId int) (map[string]common.PermissionAction, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	redisKey := fmt.Sprintf("auth:role_permissions:%d", roleId)

	// 1. Try Redis cache
	if lib.RedisClient != nil {
		cached, err := lib.RedisClient.Get(ctx, redisKey).Result()
		if err == nil && cached != "" {
			var permissions map[string]common.PermissionAction
			if jsonErr := json.Unmarshal([]byte(cached), &permissions); jsonErr == nil {
				return permissions, nil
			}
		}
	}

	// 2. Cache miss: Call Account-Service Internal API
	timestamp := time.Now().UTC().Format(time.RFC3339)
	signature, sigErr := utils.GenerateHMAC(timestamp)
	if sigErr == nil {
		url := fmt.Sprintf("%s/api/internal/roles/%d/permissions", config.Config.ServiceAccountUrl, roleId)
		resBody, reqErr := utils.InternalRequest(signature, timestamp, url, "GET", nil)
		if reqErr == nil && len(resBody) > 0 {
			var roleResp common.InternalRolePermissionsResponse
			if unmarshalErr := json.Unmarshal(resBody, &roleResp); unmarshalErr == nil && roleResp.Success && roleResp.Data != nil {
				// Warm Redis cache
				if lib.RedisClient != nil {
					lib.RedisClient.Set(ctx, redisKey, string(resBody), 24*time.Hour)
				}
				return roleResp.Data, nil
			}
		} else {
			slog.Error("Failed to fetch role permissions from account-service", "error", reqErr, "roleId", roleId)
		}
	} else {
		slog.Error("Failed to generate HMAC signature for internal request", "error", sigErr)
	}

	return nil, errors.New("failed to retrieve role permissions: account-service unreachable")
}

type FeatureAction struct {
	Feature string
	Action  string
}

func RequireAnyPermission(perms ...FeatureAction) fiber.Handler {
	return func(c *fiber.Ctx) error {
		claims, err := validateToken(c)
		if err != nil {
			var appErr *response.AppError
			if errors.As(err, &appErr) {
				return err
			}
			return response.Unauthorized("Unauthorized", nil)
		}

		// Super Admin full access bypass
		if strings.EqualFold(claims.Role, "admin") || claims.RoleId == 1 {
			c.Locals("user", claims)
			return c.Next()
		}

		// If roleId is missing, try default mapping
		roleId := claims.RoleId
		if roleId == 0 {
			if strings.EqualFold(claims.Role, "barista") {
				roleId = 3
			} else if strings.EqualFold(claims.Role, "user") {
				roleId = 2
			}
		}

		permissionsMap, permErr := FetchRolePermissions(roleId)
		if permErr != nil {
			return response.InternalServerError("Failed to verify authorization permissions", nil)
		}

		for _, p := range perms {
			perm, exists := permissionsMap[strings.ToLower(p.Feature)]
			if !exists {
				continue
			}

			var allowed bool
			switch strings.ToLower(p.Action) {
			case "view", "read", "get":
				allowed = perm.View
			case "create", "write", "post", "upload":
				allowed = perm.Create
			case "edit", "update", "put", "patch":
				allowed = perm.Edit
			case "delete", "remove":
				allowed = perm.Delete
			default:
				allowed = false
			}

			if allowed {
				c.Locals("user", claims)
				return c.Next()
			}
		}

		return response.Forbidden("Forbidden: You do not have permission to perform this action", nil)
	}
}

func RequirePermission(feature string, action string) fiber.Handler {
	return RequireAnyPermission(FeatureAction{Feature: feature, Action: action})
}
