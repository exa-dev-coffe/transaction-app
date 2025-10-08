package lib

import (
	"context"
	"mime/multipart"

	"eka-dev.cloud/transaction-service/config"
	"eka-dev.cloud/transaction-service/utils/response"
	"github.com/gofiber/fiber/v2/log"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

var minioClient *minio.Client

func init() {
	log.Info("Lib initialized minio")
	endpoint := config.Config.MinioEndpoint
	accessKey := config.Config.MinioAccessKey
	secretKey := config.Config.MinioSecretKey
	useSSL := config.Config.MinioUseSSL

	// Initialize minio client object.
	minioGenerateClient, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(accessKey, secretKey, ""),
		Secure: useSSL,
	})

	if err != nil {
		log.Fatal("Failed to initialize MinIO client:", err)
	}
	minioClient = minioGenerateClient

	log.Info("MinIO client initialized successfully")
}

func UploadFile(filePath string, fileHeader *multipart.FileHeader) (string, error) {
	bucketName := config.Config.MinioBucketName
	ctx := context.Background()

	file, err := fileHeader.Open()
	if err != nil {
		log.Error("Failed to open file:", err)
		return "", response.InternalServerError("Failed to open file", nil)
	}

	info, err := minioClient.PutObject(ctx, bucketName, filePath, file, fileHeader.Size, minio.PutObjectOptions{
		ContentType: fileHeader.Header.Get("Content-Type"),
	})
	if err != nil {
		log.Error("Failed to upload file to MinIO:", err)
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
		log.Error("Failed to delete file from MinIO:", err)
		return response.InternalServerError("Failed to delete file", nil)
	}

	return nil
}
