package transaction

import (
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"eka-dev.cloud/transaction-service/utils/common"
	"eka-dev.cloud/transaction-service/utils/response"
	"github.com/jmoiron/sqlx"
)

type PromotionUsageLog struct {
	TransactionID  int64
	PromotionID    int64
	MenuID         int64
	UserID         int64
	Qty            int
	DiscountAmount float64
}

type Repository interface {
	// TODO: define repository methods
	InsertThTransaction(tx *sqlx.Tx, transaction CreateTransactionRequest, voucherId *int64, discountAmount float64) (int, error)
	InsertThPosTransaction(tx *sqlx.Tx, transaction CreatePosTransactionRequest, voucherId *int64, discountAmount float64, paymentStatus string) (int, error)
	InsertTdTransaction(tx *sqlx.Tx, transactionId int, createdBy int64, data Data) error
	InsertTdTransactionBatch(tx *sqlx.Tx, transactionId int, createdBy int64, datas []Data) error
	GetListTransactionsPagination(params common.ParamsListRequest, startDate string, endDate string) (*response.Pagination[[]TransactionResponse], error)
	GetListTransactionsNoPagination(request common.ParamsListRequest, startDate string, endDate string) ([]TransactionResponse, error)
	GetOneTransaction(id int) (*TransactionResponse, error)
	GetListTransactionsByUserId(params common.ParamsListRequest, userId int64) (*response.Pagination[[]TransactionResponse], error)
	GetOneTransactionByUserId(id int, userId int64) (*TransactionResponse, error)
	UpdateOrderStatus(tx *sqlx.Tx, id int, updatedBy int64) error
	UpdatePaymentStatus(tx *sqlx.Tx, id int, status string) error
	UpdatePosPaymentMethod(tx *sqlx.Tx, id int, paymentMethod string, paymentStatus string, cashAmount float64, cashChange float64) error
	UpdatePosWalletCustomer(tx *sqlx.Tx, id int, userId int64, orderFor string) error
	UpdatePosQrisData(tx *sqlx.Tx, id int, qrString string, qrUrl string) error
	SetRatingMenu(tx *sqlx.Tx, id int, rating int, updatedBy int64) (int, error)
	SummaryReportTransactions(startDate string, endDate string) (*SummaryReportData, error)
	LogPromotionUsage(tx *sqlx.Tx, transactionId int64, promotionId int64, menuId int64, userId int64, qty int, discountAmount float64) error
	LogPromotionUsageBatch(tx *sqlx.Tx, usages []PromotionUsageLog) error
}

type transactionRepository struct {
	db *sqlx.DB
}

func NewTransactionRepository(db *sqlx.DB) Repository {
	return &transactionRepository{db: db}
}

func (r *transactionRepository) InsertThTransaction(tx *sqlx.Tx, transaction CreateTransactionRequest, voucherId *int64, discountAmount float64) (int, error) {
	var id int
	query := `INSERT INTO th_user_checkouts (user_id, table_id, order_for, total_price, created_by, voucher_id, discount_amount, order_type, payment_method, payment_status, is_cashier) 
	          VALUES ($1, $2, $3, $4, $5, $6, $7, 'DINE_IN', 'WALLET', 'PAID', FALSE) RETURNING id`

	err := tx.QueryRow(query, transaction.CreatedBy, transaction.TableId, transaction.OrderFor, transaction.Total, transaction.CreatedBy, voucherId, discountAmount).Scan(&id)
	if err != nil {
		slog.Error("Failed to insert transaction", "error", err)
		return 0, response.InternalServerError("Failed to insert transaction", nil)
	}
	return id, nil
}

func (r *transactionRepository) InsertThPosTransaction(tx *sqlx.Tx, transaction CreatePosTransactionRequest, voucherId *int64, discountAmount float64, paymentStatus string) (int, error) {
	var id int
	query := `INSERT INTO th_user_checkouts (user_id, table_id, order_for, total_price, created_by, voucher_id, discount_amount, order_type, payment_method, payment_status, cash_amount, cash_change, is_cashier) 
	          VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, TRUE) RETURNING id`

	var tableId *int64 = nil
	if transaction.TableId != nil && *transaction.TableId > 0 {
		tableId = transaction.TableId
	}

	err := tx.QueryRow(query, transaction.CreatedBy, tableId, transaction.OrderFor, transaction.Total, transaction.CreatedBy, voucherId, discountAmount, transaction.OrderType, transaction.PaymentMethod, paymentStatus, transaction.CashAmount, transaction.CashChange).Scan(&id)
	if err != nil {
		slog.Error("Failed to insert POS transaction", "error", err)
		return 0, response.InternalServerError("Failed to insert POS transaction", nil)
	}
	return id, nil
}

func (r *transactionRepository) UpdatePaymentStatus(tx *sqlx.Tx, id int, status string) error {
	query := `UPDATE th_user_checkouts SET payment_status = $1, updated_at = NOW() WHERE id = $2`
	var err error
	if tx != nil {
		_, err = tx.Exec(query, status, id)
	} else {
		_, err = r.db.Exec(query, status, id)
	}
	if err != nil {
		slog.Error("Failed to update payment status", "error", err)
		return response.InternalServerError("Failed to update payment status", nil)
	}
	return nil
}

func (r *transactionRepository) UpdatePosPaymentMethod(tx *sqlx.Tx, id int, paymentMethod string, paymentStatus string, cashAmount float64, cashChange float64) error {
	query := `UPDATE th_user_checkouts SET payment_method = $1, payment_status = $2, cash_amount = $3, cash_change = $4, updated_at = NOW() WHERE id = $5`
	var err error
	if tx != nil {
		_, err = tx.Exec(query, paymentMethod, paymentStatus, cashAmount, cashChange, id)
	} else {
		_, err = r.db.Exec(query, paymentMethod, paymentStatus, cashAmount, cashChange, id)
	}
	if err != nil {
		slog.Error("Failed to update POS payment method", "error", err)
		return response.InternalServerError("Failed to update POS payment method", nil)
	}
	return nil
}

func (r *transactionRepository) UpdatePosWalletCustomer(tx *sqlx.Tx, id int, userId int64, orderFor string) error {
	query := `UPDATE th_user_checkouts SET user_id = $1, created_by = $1, order_for = $2, updated_at = NOW() WHERE id = $3`
	var err error
	if tx != nil {
		_, err = tx.Exec(query, userId, orderFor, id)
	} else {
		_, err = r.db.Exec(query, userId, orderFor, id)
	}
	if err != nil {
		slog.Error("Failed to update POS wallet customer info", "error", err)
		return response.InternalServerError("Failed to update POS wallet customer info", nil)
	}
	return nil
}

func (r *transactionRepository) UpdatePosQrisData(tx *sqlx.Tx, id int, qrString string, qrUrl string) error {
	query := `UPDATE th_user_checkouts SET qr_string = $1, qr_url = $2, updated_at = NOW() WHERE id = $3`
	var err error
	if tx != nil {
		_, err = tx.Exec(query, qrString, qrUrl, id)
	} else {
		_, err = r.db.Exec(query, qrString, qrUrl, id)
	}
	if err != nil {
		slog.Error("Failed to update POS QRIS data", "error", err)
		return response.InternalServerError("Failed to update POS QRIS data", nil)
	}
	return nil
}

func (r *transactionRepository) InsertTdTransaction(tx *sqlx.Tx, transactionId int, createdBy int64, data Data) error {
	query := `INSERT INTO td_user_checkouts (ref_id, menu_id, qty, price, total_price, notes, created_by) VALUES ($1, $2, $3, $4, $5, $6, $7)`

	_, err := tx.Exec(query, transactionId, data.MenuID, data.Qty, data.Price, data.Total, data.Notes, createdBy)
	if err != nil {
		slog.Error("Failed to insert transaction detail", "error", err)
		return response.InternalServerError("Failed to insert transaction detail", nil)
	}
	return nil
}

func (r *transactionRepository) GetListTransactionsPagination(params common.ParamsListRequest, startDate string, endDate string) (*response.Pagination[[]TransactionResponse], error) {
	var record = make([]TransactionResponse, 0)

	common.BuildMappingField(&params, &mappingFieds)
	query := baseQuery
	if startDate != "" && endDate != "" {
		query += " WHERE CAST(t.created_at AS DATE) BETWEEN :start_date AND :end_date "
	}

	finalQuery, args := common.BuildFilterQuery(query, params, &mappingFiedType, " GROUP BY t.id ")
	args["start_date"] = startDate
	args["end_date"] = endDate

	rows, err := r.db.NamedQuery(finalQuery, args)

	if err != nil {
		slog.Error("Failed to get list transaction", "error", err)
		return nil, response.InternalServerError("Failed to get list transaction", nil)
	}
	defer func(rows *sqlx.Rows) {
		err := rows.Close()
		if err != nil {
			slog.Error("failed to close rows", "error", err)
			return
		}
	}(rows)

	for rows.Next() {
		var transaction TransactionResponse
		if err := rows.StructScan(&transaction); err != nil {
			slog.Error("Failed to scan transaction", "error", err)
			return nil, response.InternalServerError("Failed to scan transaction", nil)
		}
		record = append(record, transaction)
	}

	var totalData int

	queryCount := "SELECT COUNT(id) FROM th_user_checkouts t "

	if startDate != "" && endDate != "" {
		queryCount += " WHERE CAST(t.created_at AS DATE) BETWEEN :start_date AND :end_date "
	}

	countFinalQuery, countArgs := common.BuildCountQuery(queryCount, params, &mappingFiedType)

	countArgs["start_date"] = startDate
	countArgs["end_date"] = endDate

	countStmt, err := r.db.PrepareNamed(countFinalQuery)

	if err != nil {
		slog.Error("Failed to prepare count query", "error", err)
		return nil, response.InternalServerError("Failed to get list transaction count", nil)
	}

	defer func(countStmt *sqlx.NamedStmt) {
		err := countStmt.Close()
		if err != nil {
			slog.Error("failed to close count statement", "error", err)
			return
		}
	}(countStmt)

	if err := countStmt.Get(&totalData, countArgs); err != nil {
		slog.Error("Failed to get total data", "error", err)
		return nil, response.InternalServerError("Failed to get list transaction count", nil)
	}

	pagination := response.Pagination[[]TransactionResponse]{
		TotalData:   totalData,
		Data:        record,
		CurrentPage: params.Page,
		PageSize:    params.Size,
		TotalPages:  (totalData + params.Size - 1) / params.Size,
		LastPage:    params.Page >= (totalData+params.Size-1)/params.Size,
	}

	return &pagination, nil
}

func (r *transactionRepository) GetListTransactionsNoPagination(request common.ParamsListRequest, startDate string, endDate string) ([]TransactionResponse, error) {
	var record = make([]TransactionResponse, 0)

	common.BuildMappingField(&request, &mappingFieds)

	query := baseQuery

	if startDate != "" && endDate != "" {
		query += " WHERE CAST(t.created_at AS DATE) BETWEEN :start_date AND :end_date "
	}

	finalQuery, args := common.BuildFilterQuery(query, request, &mappingFiedType, " GROUP BY t.id ")

	args["start_date"] = startDate
	args["end_date"] = endDate

	rows, err := r.db.NamedQuery(finalQuery, args)
	if err != nil {
		slog.Error("Failed to get list transaction", "error", err)
		return nil, response.InternalServerError("Failed to get list transaction", nil)
	}
	defer func(rows *sqlx.Rows) {
		err := rows.Close()
		if err != nil {
			slog.Error("failed to close rows", "error", err)
			return
		}
	}(rows)

	for rows.Next() {
		var transaction TransactionResponse
		if err := rows.StructScan(&transaction); err != nil {
			slog.Error("Failed to scan transaction", "error", err)
			return nil, response.InternalServerError("Failed to scan transaction", nil)
		}
		record = append(record, transaction)
	}

	return record, nil
}

func (r *transactionRepository) GetOneTransaction(id int) (*TransactionResponse, error) {
	var record TransactionResponse
	query := baseQuery + " WHERE t.id = $1 GROUP BY t.id "

	err := r.db.Get(&record, query, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, response.NotFound("Transaction not found", nil)
		}
		slog.Error("Failed to get transaction by ID", "error", err)
		return nil, response.InternalServerError("Failed to get transaction by ID", nil)
	}

	return &record, nil
}

func (r *transactionRepository) GetListTransactionsByUserId(params common.ParamsListRequest, userId int64) (*response.Pagination[[]TransactionResponse], error) {
	var record = make([]TransactionResponse, 0)

	common.BuildMappingField(&params, &mappingFieds)

	finalQuery, args := common.BuildFilterQuery(baseQuery+" WHERE t.user_id = :user_id ", params, &mappingFiedType, " GROUP BY t.id ")

	args["user_id"] = userId

	rows, err := r.db.NamedQuery(finalQuery, args)

	if err != nil {
		slog.Error("Failed to get list transaction", "error", err)
		return nil, response.InternalServerError("Failed to get list transaction", nil)
	}
	defer func(rows *sqlx.Rows) {
		err := rows.Close()
		if err != nil {
			slog.Error("failed to close rows", "error", err)
			return
		}
	}(rows)

	for rows.Next() {
		var transaction TransactionResponse
		if err := rows.StructScan(&transaction); err != nil {
			slog.Error("Failed to scan transaction", "error", err)
			return nil, response.InternalServerError("Failed to scan transaction", nil)
		}
		record = append(record, transaction)
	}

	var totalData int
	countFinalQuery, countArgs := common.BuildCountQuery("SELECT COUNT(id) FROM th_user_checkouts WHERE user_id = :user_id ", params, &mappingFiedType)

	countArgs["user_id"] = userId

	countStmt, err := r.db.PrepareNamed(countFinalQuery)

	if err != nil {
		slog.Error("Failed to prepare count query", "error", err)
		return nil, response.InternalServerError("Failed to get list transaction count", nil)
	}

	defer func(countStmt *sqlx.NamedStmt) {
		err := countStmt.Close()
		if err != nil {
			slog.Error("failed to close count statement", "error", err)
			return
		}
	}(countStmt)

	if err := countStmt.Get(&totalData, countArgs); err != nil {
		slog.Error("Failed to get total data", "error", err)
		return nil, response.InternalServerError("Failed to get list transaction count", nil)
	}

	pagination := response.Pagination[[]TransactionResponse]{
		TotalData:   totalData,
		Data:        record,
		CurrentPage: params.Page,
		PageSize:    params.Size,
		TotalPages:  (totalData + params.Size - 1) / params.Size,
		LastPage:    params.Page >= (totalData+params.Size-1)/params.Size,
	}

	return &pagination, nil

}

func (r *transactionRepository) GetOneTransactionByUserId(id int, userId int64) (*TransactionResponse, error) {
	var record TransactionResponse
	query := baseQuery + " WHERE t.id = $1 AND t.user_id = $2 GROUP BY t.id "

	err := r.db.Get(&record, query, id, userId)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, response.NotFound("Transaction not found", nil)
		}
		slog.Error("Failed to get transaction by ID", "error", err)
		return nil, response.InternalServerError("Failed to get transaction by ID", nil)
	}

	return &record, nil
}

func (r *transactionRepository) UpdateOrderStatus(tx *sqlx.Tx, id int, updatedBy int64) error {
	query := `UPDATE th_user_checkouts SET order_status = order_status +1, updated_by = $1 WHERE id = $2 AND order_status  < 2`

	result, err := tx.Exec(query, updatedBy, id)

	if err != nil {
		slog.Error("Failed to update order status", "error", err)
		return response.InternalServerError("Failed to update order status", nil)
	}

	err = validateAffectedRows(result, "No rows were updated, possibly due to invalid ID or order status already at maximum")

	if err != nil {
		return err
	}

	return nil
}

func (r *transactionRepository) SetRatingMenu(tx *sqlx.Tx, id int, rating int, updatedBy int64) (int, error) {
	query := `UPDATE td_user_checkouts td
		SET rating = $1, updated_by = $2
		FROM th_user_checkouts th
		WHERE td.id = $3 AND td.rating IS NULL
		AND td.ref_id = th.id AND th.order_status = 2
		RETURNING td.menu_id`

	var menuId int

	err := tx.QueryRow(query, rating, updatedBy, id).Scan(&menuId)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, response.BadRequest("No rows were updated, possibly due to invalid ID, rating already set, or order not completed", nil)
		}
		slog.Error("Failed to set rating", "error", err)
		return 0, response.InternalServerError("Failed to set rating", nil)
	}

	return menuId, nil
}

func (r *transactionRepository) SummaryReportTransactions(startDate string, endDate string) (*SummaryReportData, error) {
	result := &SummaryReportData{
		DailyData:       make([]SummaryReport, 0),
		StatusBreakdown: make([]OrderStatusBreakdown, 0),
		PeakHours:       make([]PeakHourBreakdown, 0),
		TopMenus:        make([]TopMenu, 0),
	}

	// 1. Daily Data
	queryDaily := `SELECT
		SUM(t.total_price) AS total, CAST(t.created_at AS DATE) as created_at, COUNT(t.id) AS total_order				
		FROM th_user_checkouts t
		WHERE CAST(t.created_at AS DATE) BETWEEN $1 AND $2 
		GROUP BY CAST(t.created_at AS DATE)
		ORDER BY created_at ASC`

	err := r.db.Select(&result.DailyData, queryDaily, startDate, endDate)
	if err != nil {
		slog.Error("Failed to get daily summary report", "error", err)
		return nil, response.InternalServerError("Failed to get summary report", nil)
	}

	// 2. Status Breakdown
	queryStatus := `SELECT
		order_status, COUNT(id) as count
		FROM th_user_checkouts
		WHERE CAST(created_at AS DATE) BETWEEN $1 AND $2
		GROUP BY order_status`

	err = r.db.Select(&result.StatusBreakdown, queryStatus, startDate, endDate)
	if err != nil {
		slog.Error("Failed to get status breakdown report", "error", err)
	}

	// 3. Peak Hours Breakdown
	queryPeak := `SELECT
		EXTRACT(HOUR FROM created_at) as hour, COUNT(id) as count
		FROM th_user_checkouts
		WHERE CAST(created_at AS DATE) BETWEEN $1 AND $2
		GROUP BY EXTRACT(HOUR FROM created_at)
		ORDER BY hour ASC`

	err = r.db.Select(&result.PeakHours, queryPeak, startDate, endDate)
	if err != nil {
		slog.Error("Failed to get peak hours report", "error", err)
	}

	// 4. Top 5 Menus
	queryTopMenus := `SELECT
		td.menu_id, SUM(td.qty) as total_qty
		FROM td_user_checkouts td
		JOIN th_user_checkouts t ON td.ref_id = t.id
		WHERE CAST(t.created_at AS DATE) BETWEEN $1 AND $2
		GROUP BY td.menu_id
		ORDER BY total_qty DESC
		LIMIT 5`

	err = r.db.Select(&result.TopMenus, queryTopMenus, startDate, endDate)
	if err != nil {
		slog.Error("Failed to get top menus report", "error", err)
	}

	return result, nil
}

func validateAffectedRows(info sql.Result, message string) error {
	affected, err := common.GetInfoRowsAffected(info)
	if err != nil {
		return err
	}
	if affected == 0 {
		return response.BadRequest(message, nil)
	}
	return nil
}

func (r *transactionRepository) InsertTdTransactionBatch(tx *sqlx.Tx, transactionId int, createdBy int64, datas []Data) error {
	if len(datas) == 0 {
		return nil
	}

	valueStrings := make([]string, 0, len(datas))
	valueArgs := make([]interface{}, 0, len(datas)*7)

	for i, data := range datas {
		offset := i * 7
		valueStrings = append(valueStrings, fmt.Sprintf("($%d, $%d, $%d, $%d, $%d, $%d, $%d)",
			offset+1, offset+2, offset+3, offset+4, offset+5, offset+6, offset+7))
		valueArgs = append(valueArgs, transactionId, data.MenuID, data.Qty, data.Price, data.Total, data.Notes, createdBy)
	}

	query := fmt.Sprintf("INSERT INTO td_user_checkouts (ref_id, menu_id, qty, price, total_price, notes, created_by) VALUES %s", strings.Join(valueStrings, ", "))

	var err error
	if tx != nil {
		_, err = tx.Exec(query, valueArgs...)
	} else {
		_, err = r.db.Exec(query, valueArgs...)
	}
	if err != nil {
		slog.Error("Failed to bulk insert transaction details", "error", err)
		return response.InternalServerError("Failed to bulk insert transaction details", nil)
	}
	return nil
}

func (r *transactionRepository) LogPromotionUsage(tx *sqlx.Tx, transactionId int64, promotionId int64, menuId int64, userId int64, qty int, discountAmount float64) error {
	query := `
		INSERT INTO tr_promotion_usages (transaction_id, promotion_id, menu_id, user_id, qty, discount_amount)
		VALUES ($1, $2, $3, $4, $5, $6)
	`
	var err error
	if tx != nil {
		_, err = tx.Exec(query, transactionId, promotionId, menuId, userId, qty, discountAmount)
	} else {
		_, err = r.db.Exec(query, transactionId, promotionId, menuId, userId, qty, discountAmount)
	}
	if err != nil {
		slog.Error("Failed to log promotion usage", "error", err)
		return response.InternalServerError("Failed to log promotion usage", nil)
	}
	return nil
}

func (r *transactionRepository) LogPromotionUsageBatch(tx *sqlx.Tx, usages []PromotionUsageLog) error {
	if len(usages) == 0 {
		return nil
	}

	valueStrings := make([]string, 0, len(usages))
	valueArgs := make([]interface{}, 0, len(usages)*6)

	for i, u := range usages {
		offset := i * 6
		valueStrings = append(valueStrings, fmt.Sprintf("($%d, $%d, $%d, $%d, $%d, $%d)",
			offset+1, offset+2, offset+3, offset+4, offset+5, offset+6))
		valueArgs = append(valueArgs, u.TransactionID, u.PromotionID, u.MenuID, u.UserID, u.Qty, u.DiscountAmount)
	}

	query := fmt.Sprintf("INSERT INTO tr_promotion_usages (transaction_id, promotion_id, menu_id, user_id, qty, discount_amount) VALUES %s", strings.Join(valueStrings, ", "))

	var err error
	if tx != nil {
		_, err = tx.Exec(query, valueArgs...)
	} else {
		_, err = r.db.Exec(query, valueArgs...)
	}
	if err != nil {
		slog.Error("Failed to bulk insert promotion usages", "error", err)
		return response.InternalServerError("Failed to bulk insert promotion usages", nil)
	}
	return nil
}
