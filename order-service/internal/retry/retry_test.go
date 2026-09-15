package retry

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
)

func TestRetry_SuccessOnFirst(t *testing.T) {
	calls := 0
	err := Do(context.Background(), zap.NewNop(), nil, func(ctx context.Context) error {
		calls++
		return nil
	})
	assert.NoError(t, err)
	assert.Equal(t, 1, calls)
}

func TestRetry_NonRetryableError(t *testing.T) {
	calls := 0
	err := Do(context.Background(), zap.NewNop(), nil, func(ctx context.Context) error {
		calls++
		return errors.New("some error")
	})
	assert.Error(t, err)
	assert.Equal(t, 1, calls)
}
