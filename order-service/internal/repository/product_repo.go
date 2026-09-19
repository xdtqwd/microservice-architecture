package repository

type ProductRepo struct {
	db Querier
}

func NewProductRepo(db Querier) *ProductRepo {
	return &ProductRepo{db: db}
}
