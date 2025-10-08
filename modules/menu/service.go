package menu

import (
	"eka-dev.cloud/transaction-service/modules/upload"
	"eka-dev.cloud/transaction-service/utils/common"
	"eka-dev.cloud/transaction-service/utils/response"
	"github.com/jmoiron/sqlx"
)

type Service interface {
	GetListMenusPagination(request common.ParamsListRequest) (*response.Pagination, error)
	GetListMenusNoPagination(request common.ParamsListRequest) (*[]Menu, error)
	InsertMenu(tx *sqlx.Tx, menu CreateMenuRequest) error
	UpdateMenu(tx *sqlx.Tx, menu UpdateMenuRequest) error
	DeleteMenu(tx *sqlx.Tx, request *common.OneRequest) error
	GetOneMenu(id *common.OneRequest) (*Menu, error)
	GetListMenusUncategorizedNoPagination(request common.ParamsListRequest) (*[]Menu, error)
	GetListMenusUncategorizedPagination(request common.ParamsListRequest) (*response.Pagination, error)
	SetMenuCategory(tx *sqlx.Tx, model SetMenuCategory) error
	GetMenusByCategoryID(categoryID int) (*[]Menu, error)
	UpdateMenuAvailability(tx *sqlx.Tx, model UpdateMenuAvailabilityRequest) error
}

type menuService struct {
	repo Repository
	db   *sqlx.DB
	us   upload.Service
}

func NewMenuService(repo Repository, db *sqlx.DB, us upload.Service) Service {
	return &menuService{repo: repo, db: db, us: us}
}

func (s *menuService) GetListMenusPagination(request common.ParamsListRequest) (*response.Pagination, error) {
	return s.repo.GetListMenusPagination(request)
}

func (s *menuService) GetListMenusNoPagination(request common.ParamsListRequest) (*[]Menu, error) {
	return s.repo.GetListMenusNoPagination(request)
}

func (s *menuService) InsertMenu(tx *sqlx.Tx, menu CreateMenuRequest) error {
	return s.repo.InsertMenu(tx, menu)
}

func (s *menuService) UpdateMenu(tx *sqlx.Tx, menu UpdateMenuRequest) error {
	return s.repo.UpdateMenu(tx, menu)
}

func (s *menuService) DeleteMenu(tx *sqlx.Tx, request *common.OneRequest) error {
	photo, err := s.repo.DeleteMenu(tx, request.Id)
	if err != nil {
		return err
	}
	if photo != "" {
		err = s.us.DeleteMenuFoto(photo)
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *menuService) GetOneMenu(req *common.OneRequest) (*Menu, error) {
	return s.repo.GetOneMenu(req.Id)
}

func (s *menuService) GetListMenusUncategorizedNoPagination(request common.ParamsListRequest) (*[]Menu, error) {
	return s.repo.GetListMenusUncategorizedNoPagination(request)
}

func (s *menuService) GetListMenusUncategorizedPagination(request common.ParamsListRequest) (*response.Pagination, error) {
	return s.repo.GetListMenusUncategorizedPagination(request)
}

func (s *menuService) SetMenuCategory(tx *sqlx.Tx, model SetMenuCategory) error {
	return s.repo.SetMenuCategory(tx, model)
}

func (s *menuService) GetMenusByCategoryID(categoryID int) (*[]Menu, error) {
	return s.repo.GetMenusByCategoryID(categoryID)
}

func (s *menuService) UpdateMenuAvailability(tx *sqlx.Tx, model UpdateMenuAvailabilityRequest) error {
	return s.repo.UpdateMenuAvailability(tx, model.Id, model.IsAvailable, model.UpdatedBy)
}
