//go:build integration

package repository_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"order-service/internal/repository"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestJobLease_OneClaimPerPeriod(t *testing.T) {
	ctx := context.Background()
	_, err := testPool.Exec(ctx, "UPDATE job_leases SET next_run_at = now() WHERE name = 'reconciliation'")
	require.NoError(t, err)
	lease := repository.NewJobLease(testPool)

	// три «реплики» разом — период забирает одна
	var won atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ok, err := lease.TryClaim(ctx, "reconciliation", 300*time.Millisecond)
			assert.NoError(t, err)
			if ok {
				won.Add(1)
			}
		}()
	}
	wg.Wait()
	assert.EqualValues(t, 1, won.Load(), "период забирает ровно одна реплика")

	ok, err := lease.TryClaim(ctx, "reconciliation", 300*time.Millisecond)
	require.NoError(t, err)
	assert.False(t, ok, "до конца периода — занято")

	time.Sleep(350 * time.Millisecond)
	ok, err = lease.TryClaim(ctx, "reconciliation", 300*time.Millisecond)
	require.NoError(t, err)
	assert.True(t, ok, "новый период — снова можно")
}
