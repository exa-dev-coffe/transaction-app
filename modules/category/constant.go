package category

const baseQuery = `SELECT id, name FROM tm_categories`

var mappingFieldType = map[string]string{
	"id":   "int",
	"name": "string",
}
