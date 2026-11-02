package auth

import (
	"context"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

// Revoker мгновенно обесценивает уже выданные access-токены пользователя
// (блокировка клиента, магазина, архивирование). Ключ живёт дольше access TTL.
type Revoker struct {
	rdb *redis.Client
	ttl time.Duration
}

func NewRevoker(rdb *redis.Client, accessTTL time.Duration) *Revoker {
	return &Revoker{rdb: rdb, ttl: accessTTL + time.Minute}
}

func revokeKey(id uuid.UUID) string { return "revoked:" + id.String() }

// RevokeUser: все access-токены, выпущенные до текущего момента, недействительны.
func (r *Revoker) RevokeUser(ctx context.Context, ids ...uuid.UUID) error {
	if len(ids) == 0 {
		return nil
	}
	now := strconv.FormatInt(time.Now().Unix(), 10)
	pipe := r.rdb.TxPipeline()
	for _, id := range ids {
		pipe.Set(ctx, revokeKey(id), now, r.ttl)
	}
	_, err := pipe.Exec(ctx)
	return err
}

// revokedAt возвращает момент отзыва (0 — не отзывался).
func (r *Revoker) revokedAt(ctx context.Context, id uuid.UUID) (int64, error) {
	v, err := r.rdb.Get(ctx, revokeKey(id)).Int64()
	if err == redis.Nil {
		return 0, nil
	}
	return v, err
}
