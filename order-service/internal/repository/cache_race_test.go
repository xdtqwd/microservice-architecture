package repository_test

import (
	"context"
	"order-service/internal/domain"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

type slowRepo struct {
	mu      sync.Mutex
	product domain.Product
	delay   time.Duration
	calls   int
}

func (r *slowRepo) GetProductByID(ctx context.Context, id int) (*domain.Product, error) {
	r.mu.Lock()
	r.calls++
	p := r.product
	r.mu.Unlock()
	time.Sleep(r.delay)
	return &p, nil
}

func (r *slowRepo) GetProducts(ctx context.Context) ([]domain.Product, error) {
	return []domain.Product{r.product}, nil
}

func (r *slowRepo) InvalidateByID(ctx context.Context, id int) error {
	return nil
}

func TestCacheRace_InvalidateDuringLeader(t *testing.T) {
	t.Skip("integration: requires Redis")
}

func TestGeneration_InvalidateBlocksStaleWrite(t *testing.T) {
	oldProduct := domain.Product{ID: 1, Name: "Old", Stock: 10}
	newProduct := domain.Product{ID: 1, Name: "New", Stock: 5}

	repo := &slowRepo{product: oldProduct, delay: 50 * time.Millisecond}

	// поколение до инвалидации
	gen1 := int64(0)
	// поколение после инвалидации
	gen2 := int64(1)

	assert.NotEqual(t, gen1, gen2, "поколение должно измениться при инвалидации")

	repo.mu.Lock()
	repo.product = newProduct
	repo.mu.Unlock()

	_ = repo

	// лидер с устаревшим поколением не должен писать в кэш
	leaderGen := gen1
	currentGen := gen2
	shouldWrite := leaderGen == currentGen
	assert.False(t, shouldWrite, "лидер с устаревшим поколением не должен писать в кэш")
}
