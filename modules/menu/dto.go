package menu

type CreateMenuRequest struct {
	Name        string  `db:"name" json:"name" validate:"required,min=3,max=100"`
	Description string  `db:"description" json:"description" validate:"required,min=3"`
	Price       float64 `db:"price" json:"price" validate:"required,gt=0"`
	CategoryID  *int64  `db:"category_id" json:"categoryId"`
	Photo       string  `db:"photo" json:"photo" validate:"required,url"`
	IsAvailable *bool   `db:"is_available" json:"isAvailable" validate:"required"`
	CreatedBy   int64   `db:"created_by" json:"createdBy"`
}

type UpdateMenuRequest struct {
	Id          int     `json:"id" validate:"required"`
	Name        string  `db:"name" json:"name" validate:"required,min=3,max=100"`
	Description string  `db:"description" json:"description" validate:"required,min=3"`
	Price       float64 `db:"price" json:"price" validate:"required,gt=0"`
	CategoryID  *int64  `db:"category_id" json:"categoryId"`
	Photo       string  `db:"photo" json:"photo" validate:"required,url"`
	IsAvailable *bool   `db:"is_available" json:"isAvailable" validate:"required"`
	UpdatedBy   int64   `db:"updated_by" json:"updatedBy"`
}

type SetMenuCategory struct {
	Id         int64 `db:"id" json:"id" validate:"required"`
	CategoryId int64 `db:"category_id" json:"categoryId" validate:"required"`
	UpdatedBy  int64 `db:"updated_by" json:"updatedBy"`
}

type UpdateMenuAvailabilityRequest struct {
	Id          int   `json:"id" validate:"required"`
	IsAvailable bool  `json:"isAvailable" validate:"required"`
	UpdatedBy   int64 `json:"updatedBy"`
}

type Menu struct {
	Id           int64   `db:"id" json:"id"`
	Name         string  `db:"name" json:"name"`
	Description  string  `db:"description" json:"description"`
	Photo        string  `db:"photo" json:"photo"`
	IsAvailable  bool    `db:"is_available" json:"isAvailable"`
	Price        float64 `db:"price" json:"price"`
	CategoryId   int64   `db:"category_id" json:"categoryId"`
	CategoryName string  `db:"category_name" json:"categoryName"`
}
