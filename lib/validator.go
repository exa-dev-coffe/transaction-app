package lib

import (
	"errors"
	"fmt"
	"mime/multipart"
	"regexp"
	"time"

	"eka-dev.cloud/transaction-service/utils/common"
	"eka-dev.cloud/transaction-service/utils/response"
	"github.com/go-playground/validator/v10"
	"github.com/gofiber/fiber/v2/log"
)

var Validate = validator.New()

func init() {
	log.Info("Initializing validator and registering custom validations")
	err := Validate.RegisterValidation("numeric", func(fl validator.FieldLevel) bool {
		matched, _ := regexp.MatchString(`^[0-9]{6}$`, fl.Field().String())
		return matched
	})
	Validate.RegisterStructValidation(ValidateDateOrder, common.DateOrder{})
	if err != nil {
		log.Fatal("Error registering numeric validation:", err)
	}
}

func ValidateDateOrder(sl validator.StructLevel) {
	req := sl.Current().Interface().(common.DateOrder)

	if req.StartDate == "" || req.EndDate == "" {
		return // biarkan validator `required` yang handle
	}

	layout := "2006-01-02"

	startDate, err1 := time.Parse(layout, req.StartDate)
	endDate, err2 := time.Parse(layout, req.EndDate)

	// Jika format salah
	if err1 != nil {
		sl.ReportError(req.StartDate, "StartDate", "start_date", "invalid_date_format", "format harus YYYY-MM-DD")
		return
	}
	if err2 != nil {
		sl.ReportError(req.EndDate, "EndDate", "end_date", "invalid_date_format", "format harus YYYY-MM-DD")
		return
	}

	// Jika endDate sebelum startDate
	if endDate.Before(startDate) {
		sl.ReportError(req.EndDate, "EndDate", "end_date", "dateorder", "EndDate tidak boleh sebelum StartDate")
	}
}

func formatValidationError(err error) map[string]string {
	errorsMap := make(map[string]string)
	var errs validator.ValidationErrors
	if errors.As(err, &errs) {
		for _, e := range errs {
			fieldName := e.Field() // default pakai nama struct field
			// ambil nama dari json tag kalau ada
			if jsonTag := e.StructField(); jsonTag != "" {
				fieldName = e.Field()
			}
			errorsMap[fieldName] = validationMessage(e)
		}
	}
	return errorsMap
}

func validateStruct(s interface{}) error {
	return Validate.Struct(s)
}

func validationMessage(e validator.FieldError) string {
	switch e.Tag() {
	case "required":
		return fmt.Sprintf("%s is required", e.Field())
	case "email":
		return "is not a valid email"
	case "min":
		return fmt.Sprintf("%s must be at least %s characters", e.Field(), e.Param())
	case "max":
		return fmt.Sprintf("%s must be at most %s characters", e.Field(), e.Param())
	case "gte":
		return fmt.Sprintf("%s must be greater than or equal to %s", e.Field(), e.Param())
	case "lte":
		return fmt.Sprintf("%s must be less than or equal to %s", e.Field(), e.Param())
	case "oneof":
		return fmt.Sprintf("%s must be one of [%s]", e.Field(), e.Param())
	case "numeric":
		return fmt.Sprintf("%s must be a 6-digit numeric code", e.Field())
	case "invalid_date_format":
		return fmt.Sprintf("%s must be in YYYY-MM-DD format", e.Field())
	case "dateorder":
		return fmt.Sprintf("%s must be after StartDate", e.Field())
	default:
		return fmt.Sprintf("%s is not valid", e.Field())
	}
}

func ValidateRequest(s interface{}) error {
	err := validateStruct(s)
	if err != nil {
		return response.BadRequest("Validation error", formatValidationError(err))
	}
	return nil
}

func ValidateImageFile(fileHeader *multipart.FileHeader) (*multipart.FileHeader, error) {
	if fileHeader == nil {
		return nil, response.BadRequest("File is required", nil)
	}

	// Check file size (e.g., max 5MB)
	const maxFileSize = 5 * 1024 * 1024 // 5MB
	if fileHeader.Size > maxFileSize {
		return nil, response.BadRequest("File size exceeds the maximum limit of 5MB", nil)
	}

	// Check file type
	allowedTypes := map[string]bool{
		"image/jpeg": true,
		"image/png":  true,
		"image/gif":  true,
	}

	if !allowedTypes[fileHeader.Header.Get("Content-Type")] {
		return nil, response.BadRequest("Invalid file type. Only JPEG, PNG, and GIF are allowed", nil)
	}

	return fileHeader, nil
}
