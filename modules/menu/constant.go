package menu

const baseQuery = `SELECT m.id, m.name, m.description, m.price, m.photo, m.is_available, COALESCE(c.id, 0) AS category_id, COALESCE(c.name, 'Uncategorized') AS category_name FROM tm_menus m
LEFT JOIN tm_categories c ON m.category_id = c.id`

const baseQueryUncategorized = `SELECT m.id, m.name, m.description, m.price, m.photo, m.is_available, COALESCE(c.id, 0) AS category_id, COALESCE(c.name, 'Uncategorized') AS category_name FROM tm_menus m
	LEFT JOIN tm_categories c ON m.category_id = c.id WHERE m.category_id IS NULL`

var mappingFieds = map[string]string{
	"id":            "m.id",
	"name":          "m.name",
	"price":         "m.price",
	"category_name": "c.name",
	"category_id":   "c.id",
}

var errorConstraint = map[string]string{
	"tm_menus_category_id_fkey": "Category not found",
}

var mappingFieldType = map[string]string{
	"m.id":    "int",
	"m.name":  "string",
	"m.price": "int",
	"c.name":  "string",
	"c.id":    "int",
}
