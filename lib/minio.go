package lib

import (
	"context"
	"log/slog"
	"mime/multipart"

	"eka-dev.cloud/transaction-service/config"
	"eka-dev.cloud/transaction-service/utils/response"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

var minioClient *minio.Client

func init() {
	slog.Info("Lib initialized minio")
	endpoint := config.Config.MinioEndpoint
	if endpoint == "" {
		slog.Warn("MinIO endpoint is empty, skipping client initialization")
		return
	}
	accessKey := config.Config.MinioAccessKey
	secretKey := config.Config.MinioSecretKey
	useSSL := config.Config.MinioUseSSL

	// Initialize minio client object.
	minioGenerateClient, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(accessKey, secretKey, ""),
		Secure: useSSL,
	})

	if err != nil {
		slog.Error("Failed to initialize MinIO client", "error", err)
		return
	}
	minioClient = minioGenerateClient

	slog.Info("MinIO client initialized successfully")
}

func UploadFile(filePath string, fileHeader *multipart.FileHeader) (string, error) {
	bucketName := config.Config.MinioBucketName
	ctx := context.Background()

	file, err := fileHeader.Open()
	if err != nil {
		slog.Error("Failed to open file", "error", err)
		return "", response.InternalServerError("Failed to open file", nil)
	}

	info, err := minioClient.PutObject(ctx, bucketName, filePath, file, fileHeader.Size, minio.PutObjectOptions{
		ContentType: fileHeader.Header.Get("Content-Type"),
	})
	if err != nil {
		slog.Error("Failed to upload file to MinIO", "error", err)
		return "", response.InternalServerError("Failed to upload file", nil)
	}

	url := config.Config.MinioBaseURL + "/" + bucketName + "/" + info.Key
	return url, nil
}

func DeleteFile(filePath string) error {
	bucketName := config.Config.MinioBucketName

	ctx := context.Background()

	err := minioClient.RemoveObject(ctx, bucketName, filePath, minio.RemoveObjectOptions{})
	if err != nil {
		slog.Error("Failed to delete file from MinIO", "error", err)
		return response.InternalServerError("Failed to delete file", nil)
	}

	return nil
}
