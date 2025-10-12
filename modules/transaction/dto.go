package transaction

// TODO: define DTOs here

type InternalMenuResponse struct {
	Data []MenuResponse `json:"data"`
}

type MenuResponse struct {
	Id    int     `json:"id" db:"id"`
	Price float64 `json:"price" db:"price"`
}

type Data struct {
	MenuID int     `json:"menuId" validate:"required"`
	Qty    int     `json:"qty" validate:"required,gt=0"`
	Notes  string  `json:"notes" `
	Price  float64 `json:"price"`
	Total  float64 `json:"total"`
}

type CreateTransactionRequest struct {
	TableId   int64   `json:"tableId" validate:"required"`
	OrderFor  string  `json:"orderFor" validate:"required"`
	Pin       string  `json:"pin" validate:"required,len=6,numeric"`
	Datas     []Data  `json:"datas" validate:"required,dive,required"`
	Total     float64 `json:"total"`
	CreatedBy int64   `json:"createdBy"`
}

type PaymentRequest struct {
	UserId int64   `json:"userId"`
	Amount float64 `json:"amount" `
	Pin    string  `json:"pin"`
}
