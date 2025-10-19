package common

import (
	"github.com/golang-jwt/jwt/v5"
)

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

type InternalResponse struct {
	Message string `json:"message"`
	Success bool   `json:"success"`
}

type Claims struct {
	FullName string `json:"FullName"`
	Email    string `json:"Email"`
	UserId   int64  `json:"UserId"`
	Type     string `json:"Type"`
	Role     string `json:"Role"`
	jwt.RegisteredClaims
}

type DateOrder struct {
	StartDate string `json:"startDate" validate:"required" `
	EndDate   string `json:"endDate" validate:"required" `
}
