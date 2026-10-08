package voucher

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"eka-dev.cloud/transaction-service/config"
	"eka-dev.cloud/transaction-service/lib"
	"eka-dev.cloud/transaction-service/utils/common"
	"eka-dev.cloud/transaction-service/utils/response"
	"github.com/hibiken/asynq"
	"github.com/jmoiron/sqlx"
)

type Service interface {
	ValidateVoucher(ctx context.Context, request ValidateVoucherRequest, userId int64) (*ValidateVoucherResponse, error)
	ValidateVoucherForCheckout(ctx context.Context, tx *sqlx.Tx, code string, orderTotal float64, userId int64) (int64, float64, error)
	LogVoucherUsage(ctx context.Context, tx *sqlx.Tx, userId int64, voucherId int64, checkoutId int64, discountAmount float64) error
	DeactivateVoucher(ctx context.Context, tx *sqlx.Tx, id int64) error
	CreateVoucher(ctx context.Context, tx *sqlx.Tx, request CreateVoucherRequest) (int64, error)
	GetListVouchers(ctx context.Context, params common.ParamsListRequest, isPublicOnly bool, userId int64) (*response.Pagination[[]Voucher], error)
	DeleteVoucher(ctx context.Context, tx *sqlx.Tx, id int64) error
	UpdateVoucherStatus(ctx context.Context, tx *sqlx.Tx, id int64, isActive *bool, isPublic *bool) error
}

type voucherService struct {
	repo Repository
	db   *sqlx.DB
}

func NewVoucherService(repo Repository, db *sqlx.DB) Service {
	return &voucherService{repo: repo, db: db}
}

func (s *voucherService) ValidateVoucher(ctx context.Context, request ValidateVoucherRequest, userId int64) (*ValidateVoucherResponse, error) {
	voucher, err := s.repo.GetVoucherByCode(ctx, nil, request.Code)
	if err != nil {
		var appErr *response.AppError
		if errors.As(err, &appErr) && appErr.Code == http.StatusNotFound {
			return &ValidateVoucherResponse{Valid: false, Message: "Voucher not found or expired"}, nil
		}
		return nil, err
	}

	if request.OrderTotal < voucher.MinPurchase {
		return &ValidateVoucherResponse{
			Valid:   false,
			Message: fmt.Sprintf("Minimum purchase of %s is not met for voucher %s", formatRupiah(voucher.MinPurchase), voucher.Code),
		}, nil
	}

	usageCount, err := s.repo.CheckUserVoucherUsage(ctx, nil, userId, voucher.ID)
	if err != nil {
		return nil, err
	}
	if usageCount > 0 {
		return &ValidateVoucherResponse{
			Valid:   false,
			Message: "You have already used this voucher",
		}, nil
	}

	if voucher.Quota == 0 {
		return &ValidateVoucherResponse{
			Valid:   false,
			Message: "Voucher quota has been reached",
		}, nil
	}

	var discountAmount float64 = 0
	if voucher.DiscountType == "PERCENTAGE" {
		discountAmount = request.OrderTotal * (voucher.DiscountValue / 100)
		if voucher.MaxDiscount > 0 && discountAmount > voucher.MaxDiscount {
			discountAmount = voucher.MaxDiscount
		}
	} else {
		discountAmount = voucher.DiscountValue
	}

	if discountAmount > request.OrderTotal {
		discountAmount = request.OrderTotal
	}

	return &ValidateVoucherResponse{
		Valid:          true,
		DiscountAmount: discountAmount,
		FinalTotal:     request.OrderTotal - discountAmount,
		Message:        "Voucher applied successfully",
		DiscountType:   voucher.DiscountType,
		DiscountValue:  voucher.DiscountValue,
		MaxDiscount:    voucher.MaxDiscount,
		MinPurchase:    voucher.MinPurchase,
	}, nil
}

func (s *voucherService) ValidateVoucherForCheckout(ctx context.Context, tx *sqlx.Tx, code string, orderTotal float64, userId int64) (int64, float64, error) {
	voucher, err := s.repo.GetVoucherByCode(ctx, tx, code)
	if err != nil {
		return 0, 0, err
	}

	if orderTotal < voucher.MinPurchase {
		return 0, 0, response.BadRequest(fmt.Sprintf("Minimum purchase of %s is not met for voucher %s", formatRupiah(voucher.MinPurchase), voucher.Code), nil)
	}

	usageCount, err := s.repo.CheckUserVoucherUsage(ctx, tx, userId, voucher.ID)
	if err != nil {
		return 0, 0, err
	}
	if usageCount > 0 {
		return 0, 0, response.BadRequest("You have already used this voucher", nil)
	}

	if voucher.Quota == 0 {
		return 0, 0, response.BadRequest("Voucher quota has been reached", nil)
	}

	var discountAmount float64 = 0
	if voucher.DiscountType == "PERCENTAGE" {
		discountAmount = orderTotal * (voucher.DiscountValue / 100)
		if voucher.MaxDiscount > 0 && discountAmount > voucher.MaxDiscount {
			discountAmount = voucher.MaxDiscount
		}
	} else {
		discountAmount = voucher.DiscountValue
	}

	if discountAmount > orderTotal {
		discountAmount = orderTotal
	}

	return voucher.ID, discountAmount, nil
}

func (s *voucherService) LogVoucherUsage(ctx context.Context, tx *sqlx.Tx, userId int64, voucherId int64, checkoutId int64, discountAmount float64) error {
	err := s.repo.InsertVoucherUsage(ctx, tx, userId, voucherId, checkoutId, discountAmount)
	if err != nil {
		return err
	}
	return s.repo.DecrementVoucherQuota(ctx, tx, voucherId)
}

func (s *voucherService) DeactivateVoucher(ctx context.Context, tx *sqlx.Tx, id int64) error {
	return s.repo.DeactivateVoucher(ctx, tx, id)
}

func (s *voucherService) CreateVoucher(ctx context.Context, tx *sqlx.Tx, request CreateVoucherRequest) (int64, error) {
	if request.DiscountType == "PERCENTAGE" && request.DiscountValue > 100 {
		return 0, response.BadRequest("Percentage discount value cannot exceed 100%", nil)
	}

	id, err := s.repo.InsertVoucher(ctx, tx, request)
	if err != nil {
		return 0, err
	}

	// Schedule Asynq task for deactivation
	expireTime, err := parseTime(request.ExpiredAt)
	if err != nil {
		slog.Error("Failed to parse expired_at time for Asynq scheduling", "error", err)
		return id, nil
	}

	payload, err := json.Marshal(map[string]string{
		"url":  fmt.Sprintf("%s/api/1.0/internal/vouchers/deactivate", config.Config.ServiceTransactionUrl),
		"body": fmt.Sprintf(`{"id": %d}`, id),
	})
	if err != nil {
		slog.Error("Failed to marshal Asynq task payload", "error", err)
		return id, nil
	}

	if lib.AsynqClient != nil {
		task := asynq.NewTask("task:http_post", payload)
		_, err = lib.AsynqClient.Enqueue(task, asynq.ProcessAt(expireTime))
		if err != nil {
			slog.Error("Failed to enqueue deactivation task in Asynq", "error", err)
		} else {
			slog.Info("Scheduled deactivation task for Voucher", "voucher_id", id, "time", expireTime.Format(time.RFC3339))
		}
	}

	return id, nil
}

func (s *voucherService) GetListVouchers(ctx context.Context, params common.ParamsListRequest, isPublicOnly bool, userId int64) (*response.Pagination[[]Voucher], error) {
	return s.repo.ListVouchers(ctx, params, isPublicOnly, userId)
}

func (s *voucherService) DeleteVoucher(ctx context.Context, tx *sqlx.Tx, id int64) error {
	return s.repo.DeleteVoucherByID(ctx, tx, id)
}

func (s *voucherService) UpdateVoucherStatus(ctx context.Context, tx *sqlx.Tx, id int64, isActive *bool, isPublic *bool) error {
	return s.repo.UpdateVoucherStatus(ctx, tx, id, isActive, isPublic)
}

func parseTime(tStr string) (time.Time, error) {
	formats := []string{
		time.RFC3339,
		"2006-01-02T15:04:05.000Z",
		"2006-01-02T15:04:05Z",
		"2006-01-02 15:04:05",
		"2006-01-02",
	}
	var lastErr error
	for _, f := range formats {
		t, err := time.Parse(f, tStr)
		if err == nil {
			return t, nil
		}
		lastErr = err
	}
	return time.Time{}, lastErr
}

func formatRupiah(amount float64) string {
	s := fmt.Sprintf("%.0f", amount)
	if amount < 0 {
		s = s[1:]
	}
	n := len(s)
	if n <= 3 {
		if amount < 0 {
			return "-Rp " + s
		}
		return "Rp " + s
	}
	out := make([]byte, 0, n+(n-1)/3)
	for i, c := range s {
		if i > 0 && (n-i)%3 == 0 {
			out = append(out, '.')
		}
		out = append(out, byte(c))
	}
	if amount < 0 {
		return "-Rp " + string(out)
	}
	return "Rp " + string(out)
}

