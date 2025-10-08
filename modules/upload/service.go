package upload

import (
	"fmt"
	"mime/multipart"
	"path/filepath"
	"strings"
	"time"

	"eka-dev.cloud/transaction-service/lib"
	"eka-dev.cloud/transaction-service/utils/response"
	"github.com/gofiber/fiber/v2/log"
	"github.com/google/uuid"
)

type Service interface {
	UploadMenuFoto(fileHeader *multipart.FileHeader) (*UploadResponse, error)
	DeleteMenuFoto(fileName string) error
}

type uploadService struct {
}

func NewUploadService() Service {
	return &uploadService{}
}

func generateFileName(original string) (string, error) {
	extension := filepath.Ext(original) // ambil ekstensi asli (.jpg, .png)
	u, err := uuid.NewRandom()
	if err != nil {
		log.Error("Failed to generate UUID for file name:", err)
		return "", response.InternalServerError("Failed to generate file name", nil)
	}

	fileName := fmt.Sprintf("coffe/images/menus/%s-%s%s",
		time.Now().Format("20060102150405"),
		u.String(),
		extension,
	)

	return fileName, nil
}

func extractFileNameFromURL(url string) (string, error) {
	if strings.Contains(url, "project/") {
		parts := strings.SplitAfter(url, "project/")
		if len(parts) < 2 {
			return "", response.BadRequest("Invalid URL format", nil)
		}
		return parts[1], nil
	} else {
		return "", response.BadRequest("URL does not contain expected segment", nil)
	}
}

func (s *uploadService) UploadMenuFoto(fileHeader *multipart.FileHeader) (*UploadResponse, error) {
	var res UploadResponse

	fileName, err := generateFileName(fileHeader.Filename)
	if err != nil {
		return nil, err
	}
	url, err := lib.UploadFile(fileName, fileHeader)

	if err != nil {
		return nil, err
	}
	res = UploadResponse{
		URL: url,
	}

	return &res, nil
}

func (s *uploadService) DeleteMenuFoto(url string) error {
	filepathFromUrl, err := extractFileNameFromURL(url)
	if err != nil {
		return err
	}
	err = lib.DeleteFile(filepathFromUrl)
	if err != nil {
		return err
	}
	return nil
}
