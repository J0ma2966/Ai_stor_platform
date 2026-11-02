package tryon

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/boutique-market/backend/internal/media"
)

// Retention — сроки хранения биометрии (настраиваются через TRYON_*_TTL).
// Значения по умолчанию — предложение до юридического согласования (ТЗ, раздел 20):
//   - несохранённые исходные фото: 24 ч после завершения генерации;
//   - сохранённые фото: 180 дней с последнего использования;
//   - результат без «Сохранить образ»: 30 дней;
//   - сохранённый образ: 365 дней с момента сохранения;
//   - загруженные, но ни разу не использованные фото: 24 ч.
type Retention struct {
	Photo, SavedPhoto, Result, SavedLook time.Duration
}

const orphanGrace = 24 * time.Hour
const batch = 200

// referenced — ключ нельзя удалять: он сохранён клиентом или нужен генерации,
// которая ещё не завершилась или завершилась недавно (её фото ещё в сроке).
const referencedSQL = `
	EXISTS (SELECT 1 FROM body_photos bp WHERE bp.key = k)
	OR EXISTS (SELECT 1 FROM generations g2 WHERE k = ANY(g2.photo_keys) AND g2.photos_purged_at IS NULL
	           AND g2.id <> $2 AND (g2.status IN ('queued','running') OR g2.finished_at >= $3))`

// purgeGenerationPhotos удаляет исходные фото генерации, если они больше никому не нужны,
// и отмечает photos_purged_at. recent — граница «недавно завершённых» для других генераций.
func purgeGenerationPhotos(ctx context.Context, q pgx.Tx, store media.Storage, genID uuid.UUID, keys []string, recent time.Time) error {
	rows, err := q.Query(ctx, `SELECT k FROM unnest($1::text[]) AS k WHERE NOT (`+referencedSQL+`)`, keys, genID, recent)
	if err != nil {
		return err
	}
	free, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return err
	}
	for _, k := range free {
		if err := store.Delete(ctx, k); err != nil {
			return err
		}
	}
	_, err = q.Exec(ctx, `UPDATE generations SET photos_purged_at = now() WHERE id = $1`, genID)
	return err
}

func tx(ctx context.Context, pool *pgxpool.Pool, fn func(pgx.Tx) error) error {
	t, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = t.Rollback(ctx) }()
	if err := fn(t); err != nil {
		return err
	}
	return t.Commit(ctx)
}

// PurgePhotos — исходные фото генераций, завершённых раньше now-Photo.
func PurgePhotos(ctx context.Context, pool *pgxpool.Pool, store media.Storage, r Retention, now time.Time) (int64, error) {
	cut := now.Add(-r.Photo)
	rows, err := pool.Query(ctx, `SELECT id, photo_keys FROM generations
		WHERE status IN ('succeeded','failed') AND photos_purged_at IS NULL AND finished_at < $1
		ORDER BY finished_at LIMIT $2`, cut, batch)
	if err != nil {
		return 0, err
	}
	type gen struct {
		ID   uuid.UUID
		Keys []string
	}
	gens, err := pgx.CollectRows(rows, pgx.RowToStructByPos[gen])
	if err != nil {
		return 0, err
	}
	var n int64
	for _, g := range gens {
		if err := tx(ctx, pool, func(t pgx.Tx) error {
			return purgeGenerationPhotos(ctx, t, store, g.ID, g.Keys, cut)
		}); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}

// PurgeResults — результаты с истёкшим сроком (обычные и сохранённые образы).
func PurgeResults(ctx context.Context, pool *pgxpool.Pool, store media.Storage, r Retention, now time.Time) (int64, error) {
	rows, err := pool.Query(ctx, `SELECT id, result_key FROM generations
		WHERE result_key IS NOT NULL AND result_purged_at IS NULL
		  AND ((saved_at IS NULL AND finished_at < $1) OR (saved_at IS NOT NULL AND saved_at < $2))
		LIMIT $3`, now.Add(-r.Result), now.Add(-r.SavedLook), batch)
	if err != nil {
		return 0, err
	}
	type gen struct {
		ID  uuid.UUID
		Key string
	}
	gens, err := pgx.CollectRows(rows, pgx.RowToStructByPos[gen])
	if err != nil {
		return 0, err
	}
	var n int64
	for _, g := range gens {
		// сначала объект, потом ссылка: при сбое следующий запуск повторит
		if err := store.Delete(ctx, g.Key); err != nil {
			return n, err
		}
		if _, err := pool.Exec(ctx, `UPDATE generations SET result_key = NULL, result_purged_at = now(), saved_at = NULL
			WHERE id = $1`, g.ID); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}

// PurgeSavedPhotos — сохранённые фото, которыми не пользовались дольше SavedPhoto.
func PurgeSavedPhotos(ctx context.Context, pool *pgxpool.Pool, store media.Storage, r Retention, now time.Time) (int64, error) {
	rows, err := pool.Query(ctx, `DELETE FROM body_photos WHERE id IN (
		SELECT id FROM body_photos WHERE last_used_at < $1 LIMIT $2) RETURNING key`, now.Add(-r.SavedPhoto), batch)
	if err != nil {
		return 0, err
	}
	keys, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return 0, err
	}
	return deleteUnreferenced(ctx, pool, store, keys, now.Add(-r.Photo))
}

// deleteUnreferenced удаляет объекты, на которые не ссылаются сохранённые фото и
// незавершённые/недавние генерации.
func deleteUnreferenced(ctx context.Context, pool *pgxpool.Pool, store media.Storage, keys []string, recent time.Time) (int64, error) {
	if len(keys) == 0 {
		return 0, nil
	}
	rows, err := pool.Query(ctx, `SELECT k FROM unnest($1::text[]) AS k WHERE NOT (`+referencedSQL+`)`, keys, uuid.Nil, recent)
	if err != nil {
		return 0, err
	}
	free, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return 0, err
	}
	var n int64
	for _, k := range free {
		if err := store.Delete(ctx, k); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}

// PurgeOrphans — фото тела, загруженные, но не сохранённые и не использованные
// (клиент бросил примерку), и результаты, не попавшие в БД (сбой воркера).
func PurgeOrphans(ctx context.Context, pool *pgxpool.Pool, store media.Storage, r Retention, now time.Time) (int64, error) {
	var bodies, results []string
	err := store.List(ctx, "priv/u/", now.Add(-orphanGrace), func(key string) error {
		switch {
		case strings.Contains(key, "/"+string(media.PurposeBodyPhoto)+"/"):
			bodies = append(bodies, key)
		case strings.Contains(key, "/"+string(media.PurposeTryOnResult)+"/"):
			results = append(results, key)
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	var n int64
	for len(bodies) > 0 {
		b := bodies[:min(len(bodies), 500)]
		bodies = bodies[len(b):]
		// фото, когда-либо попавшее в генерацию, удаляет PurgePhotos по своим правилам
		rows, err := pool.Query(ctx, `SELECT k FROM unnest($1::text[]) AS k
			WHERE NOT EXISTS (SELECT 1 FROM body_photos bp WHERE bp.key = k)
			  AND NOT EXISTS (SELECT 1 FROM generations g WHERE k = ANY(g.photo_keys) AND g.photos_purged_at IS NULL)`, b)
		if err != nil {
			return n, err
		}
		free, err := pgx.CollectRows(rows, pgx.RowTo[string])
		if err != nil {
			return n, err
		}
		for _, k := range free {
			if err := store.Delete(ctx, k); err != nil {
				return n, err
			}
			n++
		}
	}
	for len(results) > 0 {
		b := results[:min(len(results), 500)]
		results = results[len(b):]
		rows, err := pool.Query(ctx, `SELECT k FROM unnest($1::text[]) AS k
			WHERE NOT EXISTS (SELECT 1 FROM generations g WHERE g.result_key = k)`, b)
		if err != nil {
			return n, err
		}
		free, err := pgx.CollectRows(rows, pgx.RowTo[string])
		if err != nil {
			return n, err
		}
		for _, k := range free {
			if err := store.Delete(ctx, k); err != nil {
				return n, err
			}
			n++
		}
	}
	return n, nil
}

// PurgeUser удаляет всю биометрию клиента (удаление аккаунта / отзыв согласия):
// объекты фото и результатов, сохранённые фото; строки generations остаются
// обезличенными (для журнала токенов и статистики магазинов).
func PurgeUser(ctx context.Context, t pgx.Tx, store media.Storage, userID uuid.UUID) error {
	rows, err := t.Query(ctx, `
		SELECT key FROM body_photos WHERE user_id = $1
		UNION SELECT unnest(photo_keys) FROM generations WHERE user_id = $1 AND photos_purged_at IS NULL
		UNION SELECT result_key FROM generations WHERE user_id = $1 AND result_key IS NOT NULL`, userID)
	if err != nil {
		return err
	}
	keys, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return err
	}
	for _, k := range keys {
		if err := store.Delete(ctx, k); err != nil {
			return err
		}
	}
	if _, err := t.Exec(ctx, `DELETE FROM body_photos WHERE user_id = $1`, userID); err != nil {
		return err
	}
	_, err = t.Exec(ctx, `UPDATE generations SET result_key = NULL, saved_at = NULL,
		photos_purged_at = COALESCE(photos_purged_at, now()), result_purged_at = COALESCE(result_purged_at, now()),
		deleted_at = COALESCE(deleted_at, now()), body = '{}'::jsonb
		WHERE user_id = $1`, userID)
	return err
}
