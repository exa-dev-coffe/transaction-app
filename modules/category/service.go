package category

import (
	"eka-dev.cloud/transaction-service/utils/common"
	"eka-dev.cloud/transaction-service/utils/response"
	"github.com/jmoiron/sqlx"
)

type Service interface {
	GetListCategoriesPagination(request common.ParamsListRequest) (*response.Pagination, error)
	GetListCategoriesNoPagination(request common.ParamsListRequest) (*[]Category, error)
	InsertCategory(tx *sqlx.Tx, category CreateCategoryRequest) error
	DeleteCategory(tx *sqlx.Tx, request *common.OneRequest) error
}
type categoryService struct {
	repo Repository
	db   *sqlx.DB
}

func NewCategoryService(repo Repository, db *sqlx.DB) Service {
	return &categoryService{repo: repo, db: db}
}

func (s *categoryService) GetListCategoriesPagination(request common.ParamsListRequest) (*response.Pagination, error) {
	return s.repo.GetListCategoriesPagination(request)
}

func (s *categoryService) GetListCategoriesNoPagination(request common.ParamsListRequest) (*[]Category, error) {
	return s.repo.GetListCategoriesNoPagination(request)
}

func (s *categoryService) InsertCategory(tx *sqlx.Tx, category CreateCategoryRequest) error {
	return s.repo.InsertCategory(tx, category)
}

func (s *categoryService) DeleteCategory(tx *sqlx.Tx, request *common.OneRequest) error {
	return s.repo.DeleteCategory(tx, request.Id)
}
