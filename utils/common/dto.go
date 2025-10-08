package common

type ParamsListRequest struct {
	Search     Search // field, value
	Sort       Sort   // field, order
	Size       int
	Page       int
	NoPaginate bool
}
type Search struct {
	Field []string
	Value []string
}

type Sort struct {
	Field string
	Order string
}

type OneRequest struct {
	Id int `query:"id" validate:"required"`
}

type DeleteImageRequest struct {
	Url string `query:"url" validate:"required"`
}

type GetByIDRequest struct {
	Id int `json:"id" validate:"required"`
}
