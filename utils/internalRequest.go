package utils

import (
	"encoding/json"
	"io"
	"net/http"
	"time"

	"eka-dev.cloud/transaction-service/utils/common"
	"eka-dev.cloud/transaction-service/utils/response"
	"github.com/gofiber/fiber/v2/log"
)

func InternalRequest(signature string, timestamp string, url string, method string, body io.Reader) ([]byte, error) {
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		log.Error("Failed to create request:", err)
		return nil, response.InternalServerError("Internal Server Error", nil)
	}

	req.Header.Add("x-signature", signature)
	req.Header.Add("x-timestamp", timestamp)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	res, err := client.Do(req)
	if err != nil {
		log.Error("Failed to send request:", err)
		return nil, response.InternalServerError("Internal Server Error", nil)
	}
	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {
			log.Error("Failed to close response body:", err)
		}
	}(res.Body)

	if res.StatusCode != http.StatusOK {
		resBody, err := io.ReadAll(res.Body)
		if err != nil {
			log.Error("Failed to read response body:", err)
			return nil, response.InternalServerError("Internal Server Error", nil)
		}
		log.Errorf("Received non-OK response: %s, body: %s", res.Status, string(resBody))

		var errorResponse common.InternalResponse

		err = json.Unmarshal(resBody, &errorResponse)
		if err != nil {
			log.Error("Failed to unmarshal error response:", err)
			return nil, response.InternalServerError("Internal Server Error", nil)
		}

		return nil, response.CustomError(res.StatusCode, errorResponse.Message, nil)
	}

	//Process the response body as needed
	resBody, err := io.ReadAll(res.Body)
	if err != nil {
		log.Error("Failed to read response body:", err)
		return nil, response.InternalServerError("Internal Server Error", nil)
	}

	return resBody, nil
}
