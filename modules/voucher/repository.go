package voucher

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"eka-dev.cloud/transaction-service/utils/common"
	"eka-dev.cloud/transaction-service/utils/response"
	"github.com/gofiber/fiber/v2/log"
	"github.com/jmoiron/sqlx"
)

type Repository interface {
	GetVoucherByCode(tx *sqlx.Tx, code string) (*Voucher, error)
	InsertVoucherUsage(tx *sqlx.Tx, userId int64, voucherId int64, checkoutId int64, discountAmount float64) error
	CheckUserVoucherUsage(tx *sqlx.Tx, userId int64, voucherId int64) (int, error)
	DecrementVoucherQuota(tx *sqlx.Tx, voucherId int64) error
	DeactivateVoucher(tx *sqlx.Tx, id int64) error

	InsertVoucher(tx *sqlx.Tx, request CreateVoucherRequest) (int64, error)
	ListVouchers(params common.ParamsListRequest, isPublicOnly bool, userId int64) (*response.Pagination[[]Voucher], error)
	DeleteVoucherByID(tx *sqlx.Tx, id int64) error
	UpdateVoucherStatus(tx *sqlx.Tx, id int64, isActive *bool, isPublic *bool) error
}

type voucherRepository struct {
	db *sqlx.DB
}

func NewVoucherRepository(db *sqlx.DB) Repository {
	return &voucherRepository{db: db}
}

func (r *voucherRepository) GetVoucherByCode(tx *sqlx.Tx, code string) (*Voucher, error) {
	var voucher Voucher
	query := `SELECT id, code, discount_type, discount_value, max_discount, min_purchase, quota, is_active, is_public, expired_at, created_at, created_by, updated_at, updated_by, deleted_at FROM tm_vouchers WHERE code = $1 AND is_active = TRUE AND expired_at > NOW() AND deleted_at IS NULL`
	
	var err error
	if tx != nil {
		err = tx.Get(&voucher, query, code)
	} else {
		err = r.db.Get(&voucher, query, code)
	}

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, response.NotFound("Voucher not found or expired", nil)
		}
		log.Error("Failed to get voucher:", err)
		return nil, response.InternalServerError("Failed to get voucher", nil)
	}
	return &voucher, nil
}

func (r *voucherRepository) InsertVoucherUsage(tx *sqlx.Tx, userId int64, voucherId int64, checkoutId int64, discountAmount float64) error {
	query := `INSERT INTO tr_voucher_usages (user_id, voucher_id, checkout_id, discount_amount) VALUES ($1, $2, $3, $4)`
	var err error
	if tx != nil {
		_, err = tx.Exec(query, userId, voucherId, checkoutId, discountAmount)
	} else {
		_, err = r.db.Exec(query, userId, voucherId, checkoutId, discountAmount)
	}
	if err != nil {
		log.Error("Failed to insert voucher usage:", err)
		return response.InternalServerError("Failed to record voucher usage", nil)
	}
	return nil
}

func (r *voucherRepository) CheckUserVoucherUsage(tx *sqlx.Tx, userId int64, voucherId int64) (int, error) {
	query := `SELECT COUNT(*) FROM tr_voucher_usages WHERE user_id = $1 AND voucher_id = $2`
	var count int
	var err error
	if tx != nil {
		err = tx.Get(&count, query, userId, voucherId)
	} else {
		err = r.db.Get(&count, query, userId, voucherId)
	}
	if err != nil {
		log.Error("Failed to check user voucher usage:", err)
		return 0, response.InternalServerError("Failed to verify voucher usage history", nil)
	}
	return count, nil
}

func (r *voucherRepository) DecrementVoucherQuota(tx *sqlx.Tx, voucherId int64) error {
	query := `UPDATE tm_vouchers SET quota = CASE WHEN quota = -1 THEN -1 ELSE quota - 1 END WHERE id = $1 AND (quota > 0 OR quota = -1)`
	var err error
	if tx != nil {
		_, err = tx.Exec(query, voucherId)
	} else {
		_, err = r.db.Exec(query, voucherId)
	}
	if err != nil {
		log.Error("Failed to decrement voucher quota:", err)
		return response.InternalServerError("Failed to update voucher quota", nil)
	}
	return nil
}

func (r *voucherRepository) DeactivateVoucher(tx *sqlx.Tx, id int64) error {
	query := `UPDATE tm_vouchers SET is_active = FALSE WHERE id = $1`
	var err error
	if tx != nil {
		_, err = tx.Exec(query, id)
	} else {
		_, err = r.db.Exec(query, id)
	}
	if err != nil {
		log.Error("Failed to deactivate voucher:", err)
		return response.InternalServerError("Failed to deactivate voucher", nil)
	}
	return nil
}

func (r *voucherRepository) InsertVoucher(tx *sqlx.Tx, request CreateVoucherRequest) (int64, error) {
	isPublic := true
	if request.IsPublic != nil {
		isPublic = *request.IsPublic
	}
	query := `INSERT INTO tm_vouchers (code, discount_type, discount_value, max_discount, min_purchase, quota, is_public, expired_at, created_by) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9) RETURNING id`
	var id int64
	var err error
	if tx != nil {
		err = tx.QueryRow(query, request.Code, request.DiscountType, request.DiscountValue, request.MaxDiscount, request.MinPurchase, request.Quota, isPublic, request.ExpiredAt, request.CreatedBy).Scan(&id)
	} else {
		err = r.db.QueryRow(query, request.Code, request.DiscountType, request.DiscountValue, request.MaxDiscount, request.MinPurchase, request.Quota, isPublic, request.ExpiredAt, request.CreatedBy).Scan(&id)
	}
	if err != nil {
		if strings.Contains(err.Error(), "tm_vouchers_code_key") {
			return 0, response.BadRequest("Voucher code already exists", nil)
		}
		log.Error("Failed to insert voucher:", err)
		return 0, response.InternalServerError("Failed to create voucher", nil)
	}
	return id, nil
}

func (r *voucherRepository) ListVouchers(params common.ParamsListRequest, isPublicOnly bool, userId int64) (*response.Pagination[[]Voucher], error) {
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

	rows, err := r.db.NamedQuery(finalQuery, args)
	if err != nil {
		log.Error("Failed to get list vouchers:", err)
		return nil, response.InternalServerError("Failed to get list vouchers", nil)
	}
	defer rows.Close()

	for rows.Next() {
		var v Voucher
		if err := rows.StructScan(&v); err != nil {
			log.Error("Failed to scan voucher:", err)
			return nil, response.InternalServerError("Failed to scan voucher", nil)
		}
		record = append(record, v)
	}

	var totalData int
	finalQueryCount, argsCount := common.BuildCountQuery(queryCount, params, &voucherMappingFiedType)
	
	rowsCount, err := r.db.NamedQuery(finalQueryCount, argsCount)
	if err != nil {
		log.Error("Failed to get count vouchers:", err)
		return nil, response.InternalServerError("Failed to get count vouchers", nil)
	}
	defer rowsCount.Close()

	if rowsCount.Next() {
		if err := rowsCount.Scan(&totalData); err != nil {
			log.Error("Failed to scan count vouchers:", err)
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

func (r *voucherRepository) DeleteVoucherByID(tx *sqlx.Tx, id int64) error {
	query := `UPDATE tm_vouchers SET deleted_at = NOW(), is_active = FALSE WHERE id = $1 AND deleted_at IS NULL`
	var err error
	var info sql.Result
	if tx != nil {
		info, err = tx.Exec(query, id)
	} else {
		info, err = r.db.Exec(query, id)
	}
	if err != nil {
		log.Error("Failed to delete voucher:", err)
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

func (r *voucherRepository) UpdateVoucherStatus(tx *sqlx.Tx, id int64, isActive *bool, isPublic *bool) error {
	if isActive != nil && *isActive {
		var expired bool
		checkQuery := `SELECT expired_at <= NOW() FROM tm_vouchers WHERE id = $1 AND deleted_at IS NULL`
		var err error
		if tx != nil {
			err = tx.Get(&expired, checkQuery, id)
		} else {
			err = r.db.Get(&expired, checkQuery, id)
		}
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return response.BadRequest("Voucher not found", nil)
			}
			log.Error("Failed to check voucher expiration:", err)
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

	var err error
	var info sql.Result
	if tx != nil {
		info, err = tx.Exec(query, args...)
	} else {
		info, err = r.db.Exec(query, args...)
	}
	if err != nil {
		log.Error("Failed to update voucher status/visibility:", err)
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
