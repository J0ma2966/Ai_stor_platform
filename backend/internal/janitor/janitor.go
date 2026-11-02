// Package janitor — периодическая очистка служебных таблиц. На нескольких
// инстансах API работает только один (pg_try_advisory_lock).
package janitor

import (
	"context"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

const lockID = 774_201_001 // произвольный, уникальный для janitor

type Task struct {
	Name string
	SQL  string
}

// Tasks — что чистим. Сроки выбраны с запасом относительно бизнес-правил.
var Tasks = []Task{
	// ключ идемпотентности нужен только на время повторов клиента
	{"idempotency_keys", `DELETE FROM idempotency_keys WHERE created_at < now() - interval '48 hours'`},
	// истёкшие/отозванные refresh-токены храним неделю для расследований
	{"refresh_tokens", `DELETE FROM refresh_tokens WHERE expires_at < now() - interval '7 days'
	                     OR revoked_at < now() - interval '7 days'`},
}

// Job — задача, которой мало одного SQL (например, нужно удалить файлы в хранилище).
type Job struct {
	Name string
	Run  func(ctx context.Context) (int64, error)
}

type Janitor struct {
	DB   *pgxpool.Pool
	Log  *slog.Logger
	Jobs []Job // выполняются под тем же advisory lock после Tasks
}

// RunOnce выполняет все задачи, если удалось взять advisory lock.
func (j *Janitor) RunOnce(ctx context.Context) error {
	conn, err := j.DB.Acquire(ctx)
	if err != nil {
		return err
	}
	defer conn.Release()
	var got bool
	if err := conn.QueryRow(ctx, `SELECT pg_try_advisory_lock($1)`, lockID).Scan(&got); err != nil || !got {
		return err
	}
	defer func() { _, _ = conn.Exec(context.WithoutCancel(ctx), `SELECT pg_advisory_unlock($1)`, lockID) }()
	for _, t := range Tasks {
		tag, err := conn.Exec(ctx, t.SQL)
		if err != nil {
			j.Log.Error("janitor task failed", slog.String("task", t.Name), slog.String("err", err.Error()))
			continue
		}
		if n := tag.RowsAffected(); n > 0 {
			j.Log.Info("janitor cleaned", slog.String("task", t.Name), slog.Int64("rows", n))
		}
	}
	for _, job := range j.Jobs {
		n, err := job.Run(ctx)
		if err != nil {
			j.Log.Error("janitor job failed", slog.String("task", job.Name), slog.String("err", err.Error()))
		}
		if n > 0 {
			j.Log.Info("janitor cleaned", slog.String("task", job.Name), slog.Int64("rows", n))
		}
	}
	return nil
}

func (j *Janitor) Run(ctx context.Context, every time.Duration) {
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if err := j.RunOnce(ctx); err != nil {
				j.Log.Error("janitor", slog.String("err", err.Error()))
			}
		}
	}
}
