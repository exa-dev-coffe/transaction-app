package category

import (
	"eka-dev.cloud/transaction-service/utils/common"
	"eka-dev.cloud/transaction-service/utils/response"
	"github.com/gofiber/fiber/v2/log"
	"github.com/jmoiron/sqlx"
)

type Repository interface {
	GetListCategoriesPagination(params common.ParamsListRequest) (*response.Pagination, error)
	GetListCategoriesNoPagination(params common.ParamsListRequest) (*[]Category, error)
	InsertCategory(tx *sqlx.Tx, model CreateCategoryRequest) error
	DeleteCategory(tx *sqlx.Tx, id int) error
}

type categoryRepository struct {
	db *sqlx.DB
}

func NewCategoryRepository(db *sqlx.DB) Repository {
	return &categoryRepository{db: db}
}

func (r *categoryRepository) GetListCategoriesPagination(params common.ParamsListRequest) (*response.Pagination, error) {
	// Implementation
	var record = make([]Category, 0)

	// here
	finalQuery, args := common.BuildFilterQuery(baseQuery, params, &mappingFieldType)

	rows, err := r.db.NamedQuery(finalQuery, args)
	if err != nil {
		log.Error("Failed to execute query:", err)
		return nil, response.InternalServerError("Failed to execute query", nil)
	}

	defer func(rows *sqlx.Rows) {
		err := rows.Close()
		if err != nil {
			log.Error("failed to close rows:", err)
			return
		}
	}(rows)
	for rows.Next() {
		var category Category
		if err := rows.StructScan(&category); err != nil {
			log.Error("Failed to scan category:", err)
			return nil, response.InternalServerError("Failed to scan category", nil)
		}
		record = append(record, category)
	}

	// get total data
	var totalData int
	countQuery := `SELECT COUNT(*) FROM tm_categories`
	countFinalQuery, countArgs := common.BuildCountQuery(countQuery, params, &mappingFieldType)
	countStmt, err := r.db.PrepareNamed(countFinalQuery)

	if err != nil {
		log.Error("Failed to prepare count statement:", err)
		return nil, response.InternalServerError("Failed to prepare count statement", nil)
	}
	defer func(countStmt *sqlx.NamedStmt) {
		err := countStmt.Close()
		if err != nil {
			log.Error("failed to close count statement:", err)
			return
		}
	}(countStmt)
	err = countStmt.Get(&totalData, countArgs)
	if err != nil {
		log.Error("Failed to get total data:", err)
		return nil, response.InternalServerError("Failed to get total data", nil)
	}

	pagination := response.Pagination{
		Data:        record,
		TotalData:   totalData,
		CurrentPage: params.Page,
		PageSize:    params.Size,
		TotalPages:  (totalData + params.Size - 1) / params.Size, // Calculate total pages
		LastPage:    params.Page >= (totalData+params.Size-1)/params.Size,
	}

	return &pagination, nil

}

func (r *categoryRepository) GetListCategoriesNoPagination(params common.ParamsListRequest) (*[]Category, error) {
	var record = make([]Category, 0)

	// here
	baseQuery := `SELECT id, name FROM tm_categories`

	finalQuery, args := common.BuildFilterQuery(baseQuery, params, &mappingFieldType)

	rows, err := r.db.NamedQuery(finalQuery, args)
	if err != nil {
		log.Error("Failed to execute query:", err)
		return nil, response.InternalServerError("Failed to execute query", nil)
	}

	defer func(rows *sqlx.Rows) {
		err := rows.Close()
		if err != nil {
			log.Error("failed to close rows:", err)
			return
		}
	}(rows)
	for rows.Next() {
		var category Category
		if err := rows.StructScan(&category); err != nil {
			log.Error("Failed to scan category:", err)
			return nil, response.InternalServerError("Failed to scan category", nil)
		}
		record = append(record, category)
	}

	return &record, nil
}

func (r *categoryRepository) InsertCategory(tx *sqlx.Tx, model CreateCategoryRequest) error {
	// Implementation here
	query := `INSERT INTO tm_categories (name, created_by) VALUES ($1, $2)`
	_, err := tx.Exec(query, model.Name, model.CreatedBy)
	if err != nil {
		log.Error("Failed to insert category:", err)
		return response.InternalServerError("Failed to insert category", nil)
	}
	return nil
}

func (r *categoryRepository) DeleteCategory(tx *sqlx.Tx, id int) error {
	// Implementation here
	query := `DELETE FROM tm_categories WHERE id = $1`
	row, err := tx.Exec(query, id)
	if err != nil {
		log.Error("Failed to delete category:", err)
		return response.InternalServerError("Failed to delete category", nil)
	}
	affected, err := row.RowsAffected()
	if err != nil {
		log.Error("Failed to get affected rows:", err)
		return response.InternalServerError("Failed to get affected rows", nil)
	}
	if affected == 0 {
		return response.NotFound("Category not found", nil)
	}
	return nil
}
