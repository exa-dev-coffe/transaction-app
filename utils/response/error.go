package response

import (
	"net/http"
	"time"
)

// AppError adalah custom error untuk aplikasi
type AppError struct {
	Code      int
	Message   string      `json:"message"` // Pesan untuk client
	Data      interface{} `json:"data"`
	TimeStamp time.Time   `json:"timestamp"`
}

func (e AppError) Error() string {
	return e.Message
}

// Factory function
func NewAppError(code int, message string, data interface{}) *AppError {
	return &AppError{
		Code:      code,
		Message:   message,
		Data:      data,
		TimeStamp: time.Now(),
	}
}

// Shortcut untuk error umum
func NotFound(message string, data interface{}) *AppError {
	if message == "" {
		return NewAppError(http.StatusNotFound, "Resource not found", data)
	} else {
		return NewAppError(http.StatusNotFound, message, data)
	}
}
func BadRequest(message string, data interface{}) *AppError {
	if message == "" {
		return NewAppError(http.StatusBadRequest, "Bad request", data)
	} else {
		return NewAppError(http.StatusBadRequest, message, data)
	}
}

func Unauthorized(message string, data interface{}) *AppError {
	if message == "" {
		return NewAppError(http.StatusUnauthorized, "Unauthorized", data)
	} else {
		return NewAppError(http.StatusUnauthorized, message, data)
	}
}

func Forbidden(message string, data interface{}) *AppError {
	if message == "" {
		return NewAppError(http.StatusForbidden, "Forbidden", data)
	} else {
		return NewAppError(http.StatusForbidden, message, data)
	}
}

func InternalServerError(message string, data interface{}) *AppError {
	if message == "" {
		return NewAppError(http.StatusInternalServerError, "Internal server error", data)
	} else {
		return NewAppError(http.StatusInternalServerError, message, data)
	}
}
