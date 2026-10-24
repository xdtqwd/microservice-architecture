//go:build integration

package repository_test

import (
	"context"
	"testing"

	"order-service/internal/repository"
	"order-service/migrations"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSchemaPinger(t *testing.T) {
	latest, err := migrations.Latest()
	require.NoError(t, err)

	assert.NoError(t, repository.NewSchemaPinger(testPool, latest).Ping(context.Background()),
		"TestMain накатил все миграции — схема подходит")

	err = repository.NewSchemaPinger(testPool, latest+1).Ping(context.Background())
	assert.ErrorContains(t, err, "service needs", "образ новее схемы — не готов")
}
