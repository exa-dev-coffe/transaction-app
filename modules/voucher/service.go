package voucher

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"eka-dev.cloud/transaction-service/config"
	"eka-dev.cloud/transaction-service/lib"
	"eka-dev.cloud/transaction-service/utils/common"
	"eka-dev.cloud/transaction-service/utils/response"
	"github.com/gofiber/fiber/v2/log"
	"github.com/hibiken/asynq"
	"github.com/jmoiron/sqlx"
)

type Service interface {
	ValidateVoucher(request ValidateVoucherRequest, userId int64) (*ValidateVoucherResponse, error)
	ValidateVoucherForCheckout(tx *sqlx.Tx, code string, orderTotal float64, userId int64) (int64, float64, error)
	LogVoucherUsage(tx *sqlx.Tx, userId int64, voucherId int64, checkoutId int64, discountAmount float64) error
	DeactivateVoucher(tx *sqlx.Tx, id int64) error
	CreateVoucher(tx *sqlx.Tx, request CreateVoucherRequest) (int64, error)
	GetListVouchers(params common.ParamsListRequest) (*response.Pagination[[]Voucher], error)
	DeleteVoucher(tx *sqlx.Tx, id int64) error
	UpdateVoucherStatus(tx *sqlx.Tx, id int64, isActive bool) error
}

type voucherService struct {
	repo Repository
	db   *sqlx.DB
}

func NewVoucherService(repo Repository, db *sqlx.DB) Service {
	return &voucherService{repo: repo, db: db}
}

func (s *voucherService) ValidateVoucher(request ValidateVoucherRequest, userId int64) (*ValidateVoucherResponse, error) {
	voucher, err := s.repo.GetVoucherByCode(nil, request.Code)
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

	usageCount, err := s.repo.CheckUserVoucherUsage(nil, userId, voucher.ID)
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
	}, nil
}

func (s *voucherService) ValidateVoucherForCheckout(tx *sqlx.Tx, code string, orderTotal float64, userId int64) (int64, float64, error) {
	voucher, err := s.repo.GetVoucherByCode(tx, code)
	if err != nil {
		return 0, 0, err
	}

	if orderTotal < voucher.MinPurchase {
		return 0, 0, response.BadRequest(fmt.Sprintf("Minimum purchase of %s is not met for voucher %s", formatRupiah(voucher.MinPurchase), voucher.Code), nil)
	}

	usageCount, err := s.repo.CheckUserVoucherUsage(tx, userId, voucher.ID)
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

func (s *voucherService) LogVoucherUsage(tx *sqlx.Tx, userId int64, voucherId int64, checkoutId int64, discountAmount float64) error {
	err := s.repo.InsertVoucherUsage(tx, userId, voucherId, checkoutId, discountAmount)
	if err != nil {
		return err
	}
	return s.repo.DecrementVoucherQuota(tx, voucherId)
}

func (s *voucherService) DeactivateVoucher(tx *sqlx.Tx, id int64) error {
	return s.repo.DeactivateVoucher(tx, id)
}

func (s *voucherService) CreateVoucher(tx *sqlx.Tx, request CreateVoucherRequest) (int64, error) {
	id, err := s.repo.InsertVoucher(tx, request)
	if err != nil {
		return 0, err
	}

	// Schedule Asynq task for deactivation
	expireTime, err := parseTime(request.ExpiredAt)
	if err != nil {
		log.Error("Failed to parse expired_at time for Asynq scheduling: ", err)
		return id, nil
	}

	payload, err := json.Marshal(map[string]string{
		"url":  fmt.Sprintf("%s/api/1.0/internal/vouchers/deactivate", config.Config.ServiceTransactionUrl),
		"body": fmt.Sprintf(`{"id": %d}`, id),
	})
	if err != nil {
		log.Error("Failed to marshal Asynq task payload: ", err)
		return id, nil
	}

	task := asynq.NewTask("task:http_post", payload)
	_, err = lib.AsynqClient.Enqueue(task, asynq.ProcessAt(expireTime))
	if err != nil {
		log.Error("Failed to enqueue deactivation task in Asynq: ", err)
	} else {
		log.Infof("Scheduled deactivation task for Voucher ID %d at %s", id, expireTime.Format(time.RFC3339))
	}

	return id, nil
}

func (s *voucherService) GetListVouchers(params common.ParamsListRequest) (*response.Pagination[[]Voucher], error) {
	return s.repo.ListVouchers(params)
}

func (s *voucherService) DeleteVoucher(tx *sqlx.Tx, id int64) error {
	return s.repo.DeleteVoucherByID(tx, id)
}

func (s *voucherService) UpdateVoucherStatus(tx *sqlx.Tx, id int64, isActive bool) error {
	return s.repo.UpdateVoucherStatus(tx, id, isActive)
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

