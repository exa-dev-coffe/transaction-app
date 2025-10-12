package transaction

import (
	"eka-dev.cloud/transaction-service/utils/response"
	"github.com/gofiber/fiber/v2/log"
	"github.com/jmoiron/sqlx"
)

type Repository interface {
	// TODO: define repository methods
	InsertThTransaction(tx *sqlx.Tx, transaction CreateTransactionRequest) (int, error)
	InsertTdTransaction(tx *sqlx.Tx, transactionId int, createdBy int64, data Data) error
}

type transactionRepository struct {
	db *sqlx.DB
}

func NewTransactionRepository(db *sqlx.DB) Repository {
	return &transactionRepository{db: db}
}

func (s *transactionRepository) InsertThTransaction(tx *sqlx.Tx, transaction CreateTransactionRequest) (int, error) {
	var id int
	query := `INSERT INTO th_user_checkouts (user_id, table_id, order_for, total_price, created_by) VALUES ($1, $2, $3, $4, $5) RETURNING id`

	err := tx.QueryRow(query, transaction.CreatedBy, transaction.TableId, transaction.OrderFor, transaction.Total, transaction.CreatedBy).Scan(&id)
	if err != nil {
		log.Error("Failed to insert transaction:", err)
		return 0, response.InternalServerError("Failed to insert transaction", nil)
	}
	return id, nil
}

func (s *transactionRepository) InsertTdTransaction(tx *sqlx.Tx, transactionId int, createdBy int64, data Data) error {
	query := `INSERT INTO td_user_checkouts (ref_id, menu_id, qty, price, total_price, notes, created_by) VALUES ($1, $2, $3, $4, $5, $6, $7)`

	_, err := tx.Exec(query, transactionId, data.MenuID, data.Qty, data.Price, data.Total, data.Notes, createdBy)
	if err != nil {
		log.Error("Failed to insert transaction detail:", err)
		return response.InternalServerError("Failed to insert transaction detail", nil)
	}
	return nil
}
