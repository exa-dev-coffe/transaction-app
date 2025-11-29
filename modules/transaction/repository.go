package transaction

import (
	"database/sql"
	"errors"

	"eka-dev.cloud/transaction-service/utils/common"
	"eka-dev.cloud/transaction-service/utils/response"
	"github.com/gofiber/fiber/v2/log"
	"github.com/jmoiron/sqlx"
)

type Repository interface {
	// TODO: define repository methods
	InsertThTransaction(tx *sqlx.Tx, transaction CreateTransactionRequest) (int, error)
	InsertTdTransaction(tx *sqlx.Tx, transactionId int, createdBy int64, data Data) error
	GetListTransactionsPagination(params common.ParamsListRequest, startDate string, endDate string) (*response.Pagination[[]TransactionResponse], error)
	GetListTransactionsNoPagination(request common.ParamsListRequest, startDate string, endDate string) ([]TransactionResponse, error)
	GetOneTransaction(id int) (*TransactionResponse, error)
	GetListTransactionsByUserId(params common.ParamsListRequest, userId int64) (*response.Pagination[[]TransactionResponse], error)
	GetOneTransactionByUserId(id int, userId int64) (*TransactionResponse, error)
	UpdateOrderStatus(tx *sqlx.Tx, id int, updatedBy int64) error
	SetRatingMenu(tx *sqlx.Tx, id int, rating int, updatedBy int64) (int, error)
	SummaryReportTransactions(startDate string, endDate string) ([]SummaryReport, error)
}

type transactionRepository struct {
	db *sqlx.DB
}

func NewTransactionRepository(db *sqlx.DB) Repository {
	return &transactionRepository{db: db}
}

func (r *transactionRepository) InsertThTransaction(tx *sqlx.Tx, transaction CreateTransactionRequest) (int, error) {
	var id int
	query := `INSERT INTO th_user_checkouts (user_id, table_id, order_for, total_price, created_by) VALUES ($1, $2, $3, $4, $5) RETURNING id`

	err := tx.QueryRow(query, transaction.CreatedBy, transaction.TableId, transaction.OrderFor, transaction.Total, transaction.CreatedBy).Scan(&id)
	if err != nil {
		log.Error("Failed to insert transaction:", err)
		return 0, response.InternalServerError("Failed to insert transaction", nil)
	}
	return id, nil
}

func (r *transactionRepository) InsertTdTransaction(tx *sqlx.Tx, transactionId int, createdBy int64, data Data) error {
	query := `INSERT INTO td_user_checkouts (ref_id, menu_id, qty, price, total_price, notes, created_by) VALUES ($1, $2, $3, $4, $5, $6, $7)`

	_, err := tx.Exec(query, transactionId, data.MenuID, data.Qty, data.Price, data.Total, data.Notes, createdBy)
	if err != nil {
		log.Error("Failed to insert transaction detail:", err)
		return response.InternalServerError("Failed to insert transaction detail", nil)
	}
	return nil
}

func (r *transactionRepository) GetListTransactionsPagination(params common.ParamsListRequest, startDate string, endDate string) (*response.Pagination[[]TransactionResponse], error) {
	var record = make([]TransactionResponse, 0)

	common.BuildMappingField(params, &mappingFieds)

	query := baseQuery
	if startDate != "" && endDate != "" {
		query += " WHERE CAST(t.created_at AS DATE) BETWEEN :start_date AND :end_date "
	}

	finalQuery, args := common.BuildFilterQuery(query, params, &mappingFiedType, " GROUP BY t.id ")

	args["start_date"] = startDate
	args["end_date"] = endDate

	rows, err := r.db.NamedQuery(finalQuery, args)

	if err != nil {
		log.Error("Failed to get list transaction:", err)
		return nil, response.InternalServerError("Failed to get list transaction", nil)
	}
	defer func(rows *sqlx.Rows) {
		err := rows.Close()
		if err != nil {
			log.Error("failed to close rows:", err)
			return
		}
	}(rows)

	for rows.Next() {
		var transaction TransactionResponse
		if err := rows.StructScan(&transaction); err != nil {
			log.Error("Failed to scan transaction:", err)
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
		log.Error("Failed to prepare count query:", err)
		return nil, response.InternalServerError("Failed to get list transaction count", nil)
	}

	defer func(countStmt *sqlx.NamedStmt) {
		err := countStmt.Close()
		if err != nil {
			log.Error("failed to close count statement:", err)
			return
		}
	}(countStmt)

	if err := countStmt.Get(&totalData, countArgs); err != nil {
		log.Error("Failed to get total data:", err)
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

	common.BuildMappingField(request, &mappingFieds)

	query := baseQuery

	if startDate != "" && endDate != "" {
		query += " WHERE CAST(t.created_at AS DATE) BETWEEN :start_date AND :end_date "
	}

	finalQuery, args := common.BuildFilterQuery(query, request, &mappingFiedType, " GROUP BY t.id ")

	args["start_date"] = startDate
	args["end_date"] = endDate

	rows, err := r.db.NamedQuery(finalQuery, args)
	if err != nil {
		log.Error("Failed to get list transaction:", err)
		return nil, response.InternalServerError("Failed to get list transaction", nil)
	}
	defer func(rows *sqlx.Rows) {
		err := rows.Close()
		if err != nil {
			log.Error("failed to close rows:", err)
			return
		}
	}(rows)

	for rows.Next() {
		var transaction TransactionResponse
		if err := rows.StructScan(&transaction); err != nil {
			log.Error("Failed to scan transaction:", err)
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
		log.Error("Failed to get transaction by ID:", err)
		return nil, response.InternalServerError("Failed to get transaction by ID", nil)
	}

	return &record, nil
}

func (r *transactionRepository) GetListTransactionsByUserId(params common.ParamsListRequest, userId int64) (*response.Pagination[[]TransactionResponse], error) {
	var record = make([]TransactionResponse, 0)

	common.BuildMappingField(params, &mappingFieds)

	finalQuery, args := common.BuildFilterQuery(baseQuery+" WHERE t.user_id = :user_id ", params, &mappingFiedType, " GROUP BY t.id ")

	args["user_id"] = userId

	rows, err := r.db.NamedQuery(finalQuery, args)

	if err != nil {
		log.Error("Failed to get list transaction:", err)
		return nil, response.InternalServerError("Failed to get list transaction", nil)
	}
	defer func(rows *sqlx.Rows) {
		err := rows.Close()
		if err != nil {
			log.Error("failed to close rows:", err)
			return
		}
	}(rows)

	for rows.Next() {
		var transaction TransactionResponse
		if err := rows.StructScan(&transaction); err != nil {
			log.Error("Failed to scan transaction:", err)
			return nil, response.InternalServerError("Failed to scan transaction", nil)
		}
		record = append(record, transaction)
	}

	var totalData int
	countFinalQuery, countArgs := common.BuildCountQuery("SELECT COUNT(id) FROM th_user_checkouts WHERE user_id = :user_id ", params, &mappingFiedType)

	countArgs["user_id"] = userId

	countStmt, err := r.db.PrepareNamed(countFinalQuery)

	if err != nil {
		log.Error("Failed to prepare count query:", err)
		return nil, response.InternalServerError("Failed to get list transaction count", nil)
	}

	defer func(countStmt *sqlx.NamedStmt) {
		err := countStmt.Close()
		if err != nil {
			log.Error("failed to close count statement:", err)
			return
		}
	}(countStmt)

	if err := countStmt.Get(&totalData, countArgs); err != nil {
		log.Error("Failed to get total data:", err)
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
		log.Error("Failed to get transaction by ID:", err)
		return nil, response.InternalServerError("Failed to get transaction by ID", nil)
	}

	return &record, nil
}

func (r *transactionRepository) UpdateOrderStatus(tx *sqlx.Tx, id int, updatedBy int64) error {
	query := `UPDATE th_user_checkouts SET order_status = order_status +1, updated_by = $1 WHERE id = $2 AND order_status  < 2`

	result, err := tx.Exec(query, updatedBy, id)

	if err != nil {
		log.Error("Failed to update order status:", err)
		return response.InternalServerError("Failed to update order status", nil)
	}

	err = validateAffectedRows(result, "No rows were updated, possibly due to invalid ID or order status already at maximum")

	if err != nil {
		return err
	}

	return nil
}

func (r *transactionRepository) SetRatingMenu(tx *sqlx.Tx, id int, rating int, updatedBy int64) (int, error) {
	query := `UPDATE td_user_checkouts SET rating = $1, updated_by = $2 WHERE id = $3 AND rating IS NULL RETURNING menu_id`

	var menuId int

	err := tx.QueryRow(query, rating, updatedBy, id).Scan(&menuId)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, response.BadRequest("No rows were updated, possibly due to invalid ID or rating already set", nil)
		}
		log.Error("Failed to set rating:", err)
		return 0, response.InternalServerError("Failed to set rating", nil)
	}

	return menuId, nil
}

func (r *transactionRepository) SummaryReportTransactions(startDate string, endDate string) ([]SummaryReport, error) {
	var summary = make([]SummaryReport, 0)
	query := `SELECT
		SUM(t.total_price) AS total,  CAST(t.created_at AS DATE), COUNT(t.id) AS total_order				
		FROM th_user_checkouts t
		WHERE CAST(t.created_at AS DATE) BETWEEN $1 AND $2 group by CAST(t.created_at AS DATE)`

	rows, err := r.db.Queryx(query, startDate, endDate)
	if err != nil {
		log.Error("Failed to get summary report:", err)
		return nil, response.InternalServerError("Failed to get summary report", nil)
	}
	defer func(rows *sqlx.Rows) {
		err := rows.Close()
		if err != nil {
			log.Error("failed to close rows:", err)
			return
		}
	}(rows)

	for rows.Next() {
		var report SummaryReport
		if err := rows.StructScan(&report); err != nil {
			log.Error("Failed to scan summary report:", err)
			return nil, response.InternalServerError("Failed to scan summary report", nil)
		}
		summary = append(summary, report)
	}

	return summary, nil
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
