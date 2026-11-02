// Package audit — журнал действий администраторов и магазинов (append-only).
package audit

import (
	"context"
	"net/netip"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/boutique-market/backend/internal/auth"
	"github.com/boutique-market/backend/internal/httpx"
)

type Execer interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
}

var _ Execer = (pgx.Tx)(nil)

// Log пишет запись от имени текущего Principal. Вызывать в той же транзакции, что и действие.
func Log(ctx context.Context, q Execer, action, entityType, entityID string, data map[string]any) error {
	p, _ := auth.FromContext(ctx)
	var actor any
	var role any
	if p.Role != "" {
		actor, role = p.UserID, p.Role
	}
	var ip *netip.Addr
	if a, err := netip.ParseAddr(httpx.ClientIP(ctx)); err == nil {
		ip = &a
	}
	if data == nil {
		data = map[string]any{}
	}
	_, err := q.Exec(ctx, `
		INSERT INTO audit_log (actor_id, actor_role, action, entity_type, entity_id, data, ip, request_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7, NULLIF($8, ''))`,
		actor, role, action, entityType, entityID, data, ip, httpx.RequestID(ctx))
	return err
}
