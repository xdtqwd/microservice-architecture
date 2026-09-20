package repository_test

import (
	"context"
	"encoding/json"
	"order-service/internal/cache"
	"order-service/internal/domain"
	"order-service/internal/repository"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
)

func newTestRedis(t *testing.T) *cache.RedisCache {
	t.Helper()
	c := cache.New("localhost:6379")
	if err := c.Ping(context.Background()); err != nil {
		t.Skip("Redis unavailable:", err)
	}
	return c
}

// TestCacheRace_StaleWriteWithoutFix воспроизводит гонку на старом коде
// (без счётчика поколений) — тест должен был падать до фикса
func TestCacheRace_GenerationPreventsStaleWrite(t *testing.T) {
	c := newTestRedis(t)
	ctx := context.Background()

	oldProduct := domain.Product{ID: 99, Name: "Old", Stock: 10}
	newProduct := domain.Product{ID: 99, Name: "New", Stock: 5}

	// репозиторий с задержкой — чтобы лидер завис внутри Do
	slow := &slowRepo{
		product: oldProduct,
		delay:   100 * time.Millisecond,
	}

	// очищаем ключ перед тестом
	_ = c.Delete(ctx, "product:99")

	repo := repository.NewCachedProductRepo(slow, c, zap.NewNop())

	var wg sync.WaitGroup
	wg.Add(1)

	// горутина A: GetProductByID — войдёт в Do и зависнет на 100ms
	go func() {
		defer wg.Done()
		_, _ = repo.GetProductByID(ctx, 99)
	}()

	// горутина B: пока A внутри Do — обновляем продукт и инвалидируем
	time.Sleep(30 * time.Millisecond) // даём A войти в Do
	slow.mu.Lock()
	slow.product = newProduct
	slow.mu.Unlock()
	_ = repo.InvalidateByID(ctx, 99)

	wg.Wait()

	// проверяем что в Redis НЕТ старого значения
	var cached domain.Product
	err := c.Get(ctx, "product:99", &cached)
	if err == nil {
		// если что-то есть в кэше — должен быть новый продукт
		data, _ := json.Marshal(cached)
		t.Logf("cached value: %s", data)
		assert.Equal(t, "New", cached.Name,
			"в кэше должен быть новый продукт или пусто, не старый")
	}
	// err != nil значит кэш пуст — это тоже правильно
}
