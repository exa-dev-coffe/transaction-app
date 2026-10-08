package voucher

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"eka-dev.cloud/transaction-service/utils/common"
	"eka-dev.cloud/transaction-service/utils/response"
	"github.com/jmoiron/sqlx"
)

type Repository interface {
	GetVoucherByCode(ctx context.Context, tx *sqlx.Tx, code string) (*Voucher, error)
	InsertVoucherUsage(ctx context.Context, tx *sqlx.Tx, userId int64, voucherId int64, checkoutId int64, discountAmount float64) error
	CheckUserVoucherUsage(ctx context.Context, tx *sqlx.Tx, userId int64, voucherId int64) (int, error)
	DecrementVoucherQuota(ctx context.Context, tx *sqlx.Tx, voucherId int64) error
	DeactivateVoucher(ctx context.Context, tx *sqlx.Tx, id int64) error

	InsertVoucher(ctx context.Context, tx *sqlx.Tx, request CreateVoucherRequest) (int64, error)
	ListVouchers(ctx context.Context, params common.ParamsListRequest, isPublicOnly bool, userId int64) (*response.Pagination[[]Voucher], error)
	DeleteVoucherByID(ctx context.Context, tx *sqlx.Tx, id int64) error
	UpdateVoucherStatus(ctx context.Context, tx *sqlx.Tx, id int64, isActive *bool, isPublic *bool) error
}

type voucherRepository struct {
	db *sqlx.DB
}

func NewVoucherRepository(db *sqlx.DB) Repository {
	return &voucherRepository{db: db}
}

func (r *voucherRepository) GetVoucherByCode(ctx context.Context, tx *sqlx.Tx, code string) (*Voucher, error) {
	var voucher Voucher
	query := `SELECT id, code, discount_type, discount_value, max_discount, min_purchase, quota, is_active, is_public, expired_at, created_at, created_by, updated_at, updated_by, deleted_at FROM tm_vouchers WHERE code = $1 AND is_active = TRUE AND expired_at > NOW() AND deleted_at IS NULL`
	
	var err error
	if tx != nil {
		err = tx.GetContext(ctx, &voucher, query+" FOR UPDATE", code)
	} else {
		err = r.db.GetContext(ctx, &voucher, query, code)
	}

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, response.NotFound("Voucher not found or expired", nil)
		}
		slog.Error("Failed to get voucher", "error", err)
		return nil, response.InternalServerError("Failed to get voucher", nil)
	}
	return &voucher, nil
}

func (r *voucherRepository) InsertVoucherUsage(ctx context.Context, tx *sqlx.Tx, userId int64, voucherId int64, checkoutId int64, discountAmount float64) error {
	query := `INSERT INTO tr_voucher_usages (user_id, voucher_id, checkout_id, discount_amount) VALUES ($1, $2, $3, $4)`
	_, err := tx.ExecContext(ctx, query, userId, voucherId, checkoutId, discountAmount)
	if err != nil {
		slog.Error("Failed to insert voucher usage", "error", err)
		return response.InternalServerError("Failed to record voucher usage", nil)
	}
	return nil
}

func (r *voucherRepository) CheckUserVoucherUsage(ctx context.Context, tx *sqlx.Tx, userId int64, voucherId int64) (int, error) {
	query := `SELECT COUNT(*) FROM tr_voucher_usages WHERE user_id = $1 AND voucher_id = $2`
	var count int
	var err error
	if tx != nil {
		err = tx.GetContext(ctx, &count, query, userId, voucherId)
	} else {
		err = r.db.GetContext(ctx, &count, query, userId, voucherId)
	}
	if err != nil {
		slog.Error("Failed to check user voucher usage", "error", err)
		return 0, response.InternalServerError("Failed to verify voucher usage history", nil)
	}
	return count, nil
}

func (r *voucherRepository) DecrementVoucherQuota(ctx context.Context, tx *sqlx.Tx, voucherId int64) error {
	query := `UPDATE tm_vouchers SET quota = CASE WHEN quota = -1 THEN -1 ELSE quota - 1 END WHERE id = $1 AND (quota > 0 OR quota = -1)`
	_, err := tx.ExecContext(ctx, query, voucherId)
	if err != nil {
		slog.Error("Failed to decrement voucher quota", "error", err)
		return response.InternalServerError("Failed to update voucher quota", nil)
	}
	return nil
}

func (r *voucherRepository) DeactivateVoucher(ctx context.Context, tx *sqlx.Tx, id int64) error {
	query := `UPDATE tm_vouchers SET is_active = FALSE WHERE id = $1`
	_, err := tx.ExecContext(ctx, query, id)
	if err != nil {
		slog.Error("Failed to deactivate voucher", "error", err)
		return response.InternalServerError("Failed to deactivate voucher", nil)
	}
	return nil
}

func (r *voucherRepository) InsertVoucher(ctx context.Context, tx *sqlx.Tx, request CreateVoucherRequest) (int64, error) {
	isPublic := true
	if request.IsPublic != nil {
		isPublic = *request.IsPublic
	}
	query := `INSERT INTO tm_vouchers (code, discount_type, discount_value, max_discount, min_purchase, quota, is_public, expired_at, created_by) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9) RETURNING id`
	var id int64
	err := tx.QueryRowxContext(ctx, query, request.Code, request.DiscountType, request.DiscountValue, request.MaxDiscount, request.MinPurchase, request.Quota, isPublic, request.ExpiredAt, request.CreatedBy).Scan(&id)
	if err != nil {
		if strings.Contains(err.Error(), "tm_vouchers_code_key") {
			return 0, response.BadRequest("Voucher code already exists", nil)
		}
		slog.Error("Failed to insert voucher", "error", err)
		return 0, response.InternalServerError("Failed to create voucher", nil)
	}
	return id, nil
}

func (r *voucherRepository) ListVouchers(ctx context.Context, params common.ParamsListRequest, isPublicOnly bool, userId int64) (*response.Pagination[[]Voucher], error) {
	var record = make([]Voucher, 0)
	query := `SELECT id, code, discount_type, discount_value, max_discount, min_purchase, quota, is_active, is_public, expired_at, created_at, created_by, updated_at, updated_by, deleted_at FROM tm_vouchers WHERE deleted_at IS NULL`
	queryCount := "SELECT COUNT(id) FROM tm_vouchers WHERE deleted_at IS NULL"

	if isPublicOnly {
		query += " AND is_public = TRUE AND is_active = TRUE AND expired_at > NOW() AND quota != 0"
		queryCount += " AND is_public = TRUE AND is_active = TRUE AND expired_at > NOW() AND quota != 0"
		if userId > 0 {
			query += fmt.Sprintf(" AND id NOT IN (SELECT voucher_id FROM tr_voucher_usages WHERE user_id = %d)", userId)
			queryCount += fmt.Sprintf(" AND id NOT IN (SELECT voucher_id FROM tr_voucher_usages WHERE user_id = %d)", userId)
		}
	}

	var voucherMappingFields = map[string]string{
		"id":       "id",
		"code":     "code",
		"isPublic": "is_public",
	}
	var voucherMappingFiedType = map[string]string{
		"id":       "int",
		"code":     "string",
		"isPublic": "bool",
	}
	common.BuildMappingField(&params, &voucherMappingFields)
	finalQuery, args := common.BuildFilterQuery(query, params, &voucherMappingFiedType, "")

	rows, err := r.db.NamedQueryContext(ctx, finalQuery, args)
	if err != nil {
		slog.Error("Failed to get list vouchers", "error", err)
		return nil, response.InternalServerError("Failed to get list vouchers", nil)
	}
	defer rows.Close()

	for rows.Next() {
		var v Voucher
		if err := rows.StructScan(&v); err != nil {
			slog.Error("Failed to scan voucher", "error", err)
			return nil, response.InternalServerError("Failed to scan voucher", nil)
		}
		record = append(record, v)
	}

	var totalData int
	finalQueryCount, argsCount := common.BuildCountQuery(queryCount, params, &voucherMappingFiedType)
	
	rowsCount, err := r.db.NamedQueryContext(ctx, finalQueryCount, argsCount)
	if err != nil {
		slog.Error("Failed to get count vouchers", "error", err)
		return nil, response.InternalServerError("Failed to get count vouchers", nil)
	}
	defer rowsCount.Close()

	if rowsCount.Next() {
		if err := rowsCount.Scan(&totalData); err != nil {
			slog.Error("Failed to scan count vouchers", "error", err)
			return nil, response.InternalServerError("Failed to scan count vouchers", nil)
		}
	}

	totalPages := 1
	if params.Size > 0 {
		totalPages = (totalData + params.Size - 1) / params.Size
	}
	lastPage := params.Page >= totalPages
	pagination := response.Pagination[[]Voucher]{
		TotalData:   totalData,
		Data:        record,
		CurrentPage: params.Page,
		PageSize:    params.Size,
		TotalPages:  totalPages,
		LastPage:    lastPage,
	}
	return &pagination, nil
}

func (r *voucherRepository) DeleteVoucherByID(ctx context.Context, tx *sqlx.Tx, id int64) error {
	query := `UPDATE tm_vouchers SET deleted_at = NOW(), is_active = FALSE WHERE id = $1 AND deleted_at IS NULL`
	info, err := tx.ExecContext(ctx, query, id)
	if err != nil {
		slog.Error("Failed to delete voucher", "error", err)
		return response.InternalServerError("Failed to delete voucher", nil)
	}
	
	affected, err := common.GetInfoRowsAffected(info)
	if err != nil {
		return err
	}
	if affected == 0 {
		return response.BadRequest("Voucher not found", nil)
	}
	return nil
}

func (r *voucherRepository) UpdateVoucherStatus(ctx context.Context, tx *sqlx.Tx, id int64, isActive *bool, isPublic *bool) error {
	if isActive != nil && *isActive {
		var expired bool
		checkQuery := `SELECT expired_at <= NOW() FROM tm_vouchers WHERE id = $1 AND deleted_at IS NULL`
		err := tx.GetContext(ctx, &expired, checkQuery, id)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return response.BadRequest("Voucher not found", nil)
			}
			slog.Error("Failed to check voucher expiration", "error", err)
			return response.InternalServerError("Failed to check voucher expiration", nil)
		}
		if expired {
			return response.BadRequest("Cannot activate an expired voucher", nil)
		}
	}

	setClauses := []string{}
	args := []interface{}{}
	argIdx := 1

	if isActive != nil {
		setClauses = append(setClauses, fmt.Sprintf("is_active = $%d", argIdx))
		args = append(args, *isActive)
		argIdx++
	}

	if isPublic != nil {
		setClauses = append(setClauses, fmt.Sprintf("is_public = $%d", argIdx))
		args = append(args, *isPublic)
		argIdx++
	}

	if len(setClauses) == 0 {
		return nil
	}

	args = append(args, id)
	query := fmt.Sprintf("UPDATE tm_vouchers SET %s WHERE id = $%d AND deleted_at IS NULL", strings.Join(setClauses, ", "), argIdx)

	info, err := tx.ExecContext(ctx, query, args...)
	if err != nil {
		slog.Error("Failed to update voucher status/visibility", "error", err)
		return response.InternalServerError("Failed to update voucher status", nil)
	}

	affected, err := common.GetInfoRowsAffected(info)
	if err != nil {
		return err
	}
	if affected == 0 {
		return response.BadRequest("Voucher not found", nil)
	}
	return nil
}

