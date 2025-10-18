package transaction

const baseQuery = `
SELECT
	t.id,
	t.table_id,
	t.total_price,
	t.order_status,
	t.order_for,
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
	`

var mappingFieds = map[string]string{
	"id":          "t.id",
	"orderStatus": "t.order_status",
}
var mappingFiedType = map[string]string{
	"t.id":           "int",
	"t.order_status": "int",
}
