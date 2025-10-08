package table

type CreateTableRequest struct {
	Name      string `db:"name" json:"name" validate:"required"`
	CreatedBy int64  `db:"created_by" json:"createdBy"`
}
type UpdateTableRequest struct {
	Id        int64  `json:"id" validate:"required"`
	Name      string `db:"name" json:"name" validate:"required"`
	UpdatedBy int64  `db:"updated_by" json:"updatedBy"`
}
type Table struct {
	Id        int64  `db:"id" json:"id"`
	Name      string `db:"name" json:"name"`
	UpdatedAt string `db:"updated_at" json:"updatedAt"`
}
