package transaction

const baseQuery = `
SELECT
	t.id,
	COALESCE(t.table_id, 0) AS table_id,
	t.total_price,
	t.order_status,
	t.user_id,
	t.order_for,
	COALESCE(t.order_type, 'DINE_IN') AS order_type,
	COALESCE(t.payment_method, 'WALLET') AS payment_method,
	COALESCE(t.payment_status, 'PAID') AS payment_status,
	COALESCE(t.cash_amount, 0) AS cash_amount,
	COALESCE(t.cash_change, 0) AS cash_change,
	COALESCE(t.is_cashier, FALSE) AS is_cashier,
	COALESCE(t.qr_string, '') AS qr_string,
	COALESCE(t.qr_url, '') AS qr_url,
	t.voucher_id,
	COALESCE(MAX(v.code), '') AS voucher_code,
	t.discount_amount,
	t.created_at,
	t.updated_at,
	JSON_AGG(
        JSON_BUILD_OBJECT(            
            'menuId', td.menu_id,
            'qty', td.qty,
            'price', td.price,
            'id', td.id,
            'notes', td.notes,
            'totalPrice', td.total_price,
            'rating', td.rating                    
        )
    ) AS details
	FROM th_user_checkouts t
JOIN td_user_checkouts td ON t.id = td.ref_id
LEFT JOIN tm_vouchers v ON t.voucher_id = v.id
	`

var mappingFieds = map[string]string{
	"id":            "t.id",
	"orderStatus":   "t.order_status",
	"orderFor":      "t.order_for",
	"createdAt":     "t.created_at",
	"totalPrice":    "t.total_price",
	"isCashier":     "t.is_cashier",
	"paymentStatus": "t.payment_status",
	"paymentMethod": "t.payment_method",
}
var mappingFiedType = map[string]string{
	"t.id":             "int",
	"t.order_status":   "int",
	"t.order_for":      "string",
	"t.created_at":     "timestamp",
	"t.total_price":    "int",
	"t.is_cashier":     "bool",
	"t.payment_status": "string",
	"t.payment_method": "string",
}
