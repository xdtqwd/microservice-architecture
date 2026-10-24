package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// JobLease — право выполнить задачу в текущем периоде, одно на все реплики.
//
// SKIP LOCKED не подходит: делить нечего, задача — одно действие на весь
// сервис. Advisory lock на время работы тоже не подходит: запуски короткие
// и реплики тикают со сдвигом — к тику второй первая уже закончила. Нужно
// право на период, а не на время работы.
type JobLease struct {
	pool *pgxpool.Pool
}

func NewJobLease(pool *pgxpool.Pool) *JobLease { return &JobLease{pool: pool} }

// TryClaim забирает текущий период задачи name. true — эта реплика выполняет
// задачу, следующий период начнётся через every. Время — из базы: одни часы
// на все реплики.
func (l *JobLease) TryClaim(ctx context.Context, name string, every time.Duration) (bool, error) {
	tag, err := l.pool.Exec(ctx,
		`UPDATE job_leases
		    SET next_run_at = now() + make_interval(secs => $2)
		  WHERE name = $1 AND next_run_at <= now()`,
		name, every.Seconds())
	if err != nil {
		return false, fmt.Errorf("claim job %q: %w", name, err)
	}
	return tag.RowsAffected() == 1, nil
}
