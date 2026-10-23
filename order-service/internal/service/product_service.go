package service

import (
	"context"
	"order-service/internal/domain"

	"go.uber.org/zap"
)

type ProductService struct {
	repo   ProductRepository
	logger *zap.Logger
}

func NewProductService(repo ProductRepository, logger *zap.Logger) *ProductService {
	return &ProductService{repo: repo, logger: logger}
}

func (s *ProductService) GetProducts(ctx context.Context) ([]domain.Product, error) {
	return s.repo.GetProducts(ctx)
}

func (s *ProductService) GetProductByID(ctx context.Context, id int) (*domain.Product, error) {
	return s.repo.GetProductByID(ctx, id)
}
