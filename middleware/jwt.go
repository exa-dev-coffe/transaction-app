package middleware

import (
	"errors"
	"strings"

	"eka-dev.cloud/transaction-service/config"
	"eka-dev.cloud/transaction-service/utils/response"
	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v5"
)

type Claims struct {
	FullName string `json:"FullName"`
	Email    string `json:"Email"`
	UserId   int64  `json:"UserId"`
	Type     string `json:"Type"`
	Role     string `json:"Role"`
	jwt.RegisteredClaims
}

var jwtKey = []byte(config.Config.SecretJwt)

func getTokenFromHeader(c *fiber.Ctx) string {
	bearer := c.Get("Authorization")
	if bearer == "" {
		return ""
	}
	token := bearer[len("Bearer "):]
	return token
}

func validateToken(c *fiber.Ctx) (*Claims, error) {
	tokenString := getTokenFromHeader(c)
	if tokenString == "" {
		return nil, response.Unauthorized("Missing Token", nil)
	}

	claims := &Claims{}
	token, err := jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, response.Unauthorized("Unexpected signing method", nil)
		}
		return jwtKey, nil
	})

	if err != nil {
		// cek apakah error karena expired
		if errors.Is(err, jwt.ErrTokenExpired) {
			return nil, response.Unauthorized("Token expired", nil)
		}
		// cek error lain
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
			if userRole == role {
				c.Locals("user", claims)
				return c.Next()
			}
		}
		return response.Forbidden("Forbidden", nil)
	}
}
