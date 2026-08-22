package transaction

import (
	"encoding/json"
	"fmt"

	"eka-dev.cloud/transaction-service/utils/common"
)

// TODO: define DTOs here

type InternalMenuResponse struct {
	Data []MenuResponse `json:"data"`
}

type InternalGetMenusAndTableResponse struct {
	Data GetMenusAndTableResponse `json:"data"`
}

type GetMenusAndTableResponse struct {
	Menus  []MenuResponse  `json:"menus"`
	Tables []TableResponse `json:"tables"`
}

type DiscountDetail struct {
	PromotionID   int64   `json:"promotionId"`
	PromotionName string  `json:"promotionName"`
	DiscountType  string  `json:"discountType"`
	DiscountValue float64 `json:"discountValue"`
	Savings       float64 `json:"savings"`
}

type MenuResponse struct {
	Id             int             `json:"id" db:"id"`
	Price          float64         `json:"price" db:"price"`
	EffectivePrice float64         `json:"effectivePrice" db:"effectivePrice"`
	Discount       *DiscountDetail `json:"discount,omitempty"`
	Name           string          `json:"name" db:"name"`
	Description    string          `json:"description" db:"description"`
	Photo          string          `json:"photo" db:"photo"`
}

type TableResponse struct {
	Id   int64  `json:"id" db:"id"`
	Name string `json:"name" db:"name"`
}

type InternalGetUserResponse struct {
	Data []UserResponse `json:"data"`
}
type UserResponse struct {
	UserId   int64  `json:"userId" `
	FullName string `json:"fullName"`
	Email    string `json:"email"`
}

type Data struct {
	MenuID int     `json:"menuId" validate:"required"`
	Qty    int     `json:"qty" validate:"required,gt=0"`
	Notes  string  `json:"notes" `
	Price  float64 `json:"price"`
	Total  float64 `json:"total"`
}

type CreateTransactionRequest struct {
	TableId     int64   `json:"tableId" validate:"required"`
	OrderFor    string  `json:"orderFor" validate:"required"`
	Pin         string  `json:"pin" validate:"required,len=6,numeric"`
	VoucherCode string  `json:"voucherCode"`
	Datas       []Data  `json:"datas" validate:"required,dive,required"`
	Total       float64 `json:"total"`
	CreatedBy   int64   `json:"createdBy"`
}

type PaymentRequest struct {
	UserId int64   `json:"userId"`
	Amount float64 `json:"amount" `
	Pin    string  `json:"pin"`
}

type TransactionResponse struct {
	Id             int64                   `json:"id" db:"id"`
	OrderStatus    int8                    `json:"orderStatus" db:"order_status"`
	TotalPrice     float64                 `json:"totalPrice" db:"total_price"`
	OrderFor       string                  `json:"orderFor" db:"order_for"`
	OrderBy        string                  `json:"orderBy"`
	UserId         int64                   `json:"userId" db:"user_id"`
	TableName      string                  `json:"tableName"`
	CreatedAt      string                  `json:"createdAt" db:"created_at"`
	UpdatedAt      string                  `json:"updatedAt" db:"updated_at"`
	TableId        int64                   `json:"tableId" db:"table_id"`
	VoucherId      *int64                  `json:"voucherId" db:"voucher_id"`
	VoucherCode    string                  `json:"voucherCode" db:"voucher_code"`
	DiscountAmount float64                 `json:"discountAmount" db:"discount_amount"`
	Details        JSONBTransactionDetails `json:"details" db:"details"`
}

type JSONBTransactionDetails []TransactionDetail

func (d *JSONBTransactionDetails) Scan(value interface{}) error {
	if value == nil {
		*d = nil
		return nil
	}
	bytes, ok := value.([]byte)
	if !ok {
		return fmt.Errorf("failed to type assert value to []byte")
	}
	return json.Unmarshal(bytes, d)
}

type TransactionDetail struct {
	MenuId        int     `json:"menuId" db:"menuId"`
	Qty           int     `json:"qty" db:"qty"`
	Price         float64 `json:"price" db:"price"`
	OriginalPrice float64 `json:"originalPrice,omitempty" db:"originalPrice"`
	PromoDiscount float64 `json:"promoDiscount,omitempty" db:"promoDiscount"`
	PromotionId   *int64  `json:"promotionId,omitempty" db:"promotionId"`
	Id            int     `json:"id" db:"id"`
	Notes         string  `json:"notes" db:"notes"`
	TotalPrice    float64 `json:"totalPrice" db:"totalPrice"`
	Rating        *int8   `json:"rating" db:"rating"`
	Description   string  `json:"description" db:"description"`
	MenuName      string  `json:"menuName"`
	Photo         string  `json:"photo" db:"photo"`
}

type UpdateOrderStatusRequest struct {
	Id        int   `json:"id" validate:"required"`
	UpdatedBy int64 `json:"updatedBy"`
}

type SetRatingMenuRequest struct {
	Id        int   `json:"id" validate:"required"`
	Rating    int   `json:"rating" validate:"required,min=1,max=5"`
	UpdatedBy int64 `json:"updatedBy"`
}

type GetListTransactionsRequest struct {
	StartDate string `json:"startDate"`
	EndDate   string `json:"endDate"`
	common.ParamsListRequest
}

type SummaryReport struct {
	Total      float64 `json:"total" db:"total"`
	TotalOrder int64   `json:"totalOrder" db:"total_order"`
	CreatedAt  string  `json:"createdAt" db:"created_at"`
}

type OrderStatusBreakdown struct {
	Status int `json:"status" db:"order_status"`
	Count  int `json:"count" db:"count"`
}

type PeakHourBreakdown struct {
	Hour  int `json:"hour" db:"hour"`
	Count int `json:"count" db:"count"`
}

type TopMenu struct {
	MenuId   int `json:"menuId" db:"menu_id"`
	TotalQty int `json:"totalQty" db:"total_qty"`
}

type SummaryReportData struct {
	DailyData       []SummaryReport        `json:"dailyData"`
	StatusBreakdown []OrderStatusBreakdown `json:"statusBreakdown"`
	PeakHours       []PeakHourBreakdown    `json:"peakHours"`
	TopMenus        []TopMenu              `json:"topMenus"`
}
