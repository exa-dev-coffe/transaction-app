package voucher

type Voucher struct {
	ID            int64     `json:"id" db:"id"`
	Code          string    `json:"code" db:"code"`
	DiscountType  string    `json:"discountType" db:"discount_type"`
	DiscountValue float64   `json:"discountValue" db:"discount_value"`
	MaxDiscount   float64   `json:"maxDiscount" db:"max_discount"`
	MinPurchase   float64   `json:"minPurchase" db:"min_purchase"`
	Quota         int       `json:"quota" db:"quota"`
	IsActive      bool      `json:"isActive" db:"is_active"`
	IsPublic      bool      `json:"isPublic" db:"is_public"`
	ExpiredAt     string    `json:"expiredAt" db:"expired_at"`
	CreatedAt     string    `json:"createdAt" db:"created_at"`
	CreatedBy     *int64    `json:"createdBy" db:"created_by"`
	UpdatedAt     string    `json:"updatedAt" db:"updated_at"`
	UpdatedBy     *int64    `json:"updatedBy" db:"updated_by"`
	DeletedAt     *string   `json:"deletedAt" db:"deleted_at"`
}

type CreateVoucherRequest struct {
	Code          string  `json:"code" validate:"required,min=3,max=50"`
	DiscountType  string  `json:"discountType" validate:"required,oneof=PERCENTAGE FIXED"`
	DiscountValue float64 `json:"discountValue" validate:"required,gt=0"`
	MaxDiscount   float64 `json:"maxDiscount"`
	MinPurchase   float64 `json:"minPurchase"`
	Quota         int     `json:"quota"`
	IsPublic      *bool   `json:"isPublic"`
	ExpiredAt     string  `json:"expiredAt" validate:"required"`
	CreatedBy     int64   `json:"createdBy"`
}

type ValidateVoucherRequest struct {
	Code       string  `json:"code" validate:"required"`
	OrderTotal float64 `json:"orderTotal" validate:"required,gt=0"`
}

type ValidateVoucherResponse struct {
	Valid          bool    `json:"valid"`
	DiscountAmount float64 `json:"discountAmount"`
	FinalTotal     float64 `json:"finalTotal"`
	Message        string  `json:"message"`
	DiscountType   string  `json:"discountType,omitempty"`
	DiscountValue  float64 `json:"discountValue,omitempty"`
	MaxDiscount    float64 `json:"maxDiscount,omitempty"`
	MinPurchase    float64 `json:"minPurchase,omitempty"`
}

type UpdateVoucherStatusRequest struct {
	IsActive *bool `json:"isActive"`
	IsPublic *bool `json:"isPublic"`
}
