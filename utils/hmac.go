package utils

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"

	"eka-dev.cloud/transaction-service/config"
	"eka-dev.cloud/transaction-service/utils/response"
	"github.com/gofiber/fiber/v2/log"
)

func GenerateHMAC(message string) (string, error) {
	h := hmac.New(sha256.New, []byte(config.Config.Secret))
	h.Write([]byte(message))
	signature := base64.StdEncoding.EncodeToString(h.Sum(nil))
	return signature, nil
}

// VerifySignature memeriksa apakah signature valid
func VerifySignature(message string, signatureHeader string) error {
	// Decode base64 dari header
	signatureBytes, err := base64.StdEncoding.DecodeString(signatureHeader)
	if err != nil {
		log.Error("Failed to decode signature:", err)
		return response.InternalServerError("failed to decode signature", nil)
	}

	// Buat ulang signature dari body/message
	mac := hmac.New(sha256.New, []byte(config.Config.Secret))
	mac.Write([]byte(message))
	expectedMAC := mac.Sum(nil)

	// Bandingkan dengan waktu konstan (aman dari timing attack)
	if !hmac.Equal(signatureBytes, expectedMAC) {
		return response.Unauthorized("invalid signature", nil)
	}

	return nil
}
