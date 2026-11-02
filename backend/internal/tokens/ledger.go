// Package tokens — баланс AI-токенов. Любое изменение баланса = запись в
// append-only token_ledger + обновление users.token_balance в одной транзакции
// с блокировкой строки пользователя.
package tokens

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/boutique-market/backend/internal/platform/db"
)

type Op string

const (
	OpWelcomeBonus     Op = "welcome_bonus"
	OpPurchase         Op = "purchase"
	OpGenerationCharge Op = "generation_charge"
	OpGenerationRefund Op = "generation_refund"
	OpAdminAdjustment  Op = "admin_adjustment"
)

var (
	ErrInsufficient = errors.New("insufficient tokens")
	ErrDuplicate    = errors.New("operation already applied")
)

type Entry struct {
	UserID        uuid.UUID
	Op            Op
	Amount        int64 // >0 начисление, <0 списание
	ReferenceType string
	ReferenceID   *uuid.UUID
	Reason        string
	ActorID       *uuid.UUID
}

// Apply должен вызываться внутри транзакции вызывающего (чтобы списание и, например,
// создание генерации были атомарны). Возвращает новый баланс.
func Apply(ctx context.Context, tx pgx.Tx, e Entry) (int64, error) {
	if e.Amount == 0 {
		return 0, errors.New("tokens: zero amount")
	}
	var bal int64
	if err := tx.QueryRow(ctx, `SELECT token_balance FROM users WHERE id = $1 FOR UPDATE`, e.UserID).Scan(&bal); err != nil {
		return 0, db.NotFound(err)
	}
	newBal := bal + e.Amount
	if newBal < 0 {
		return bal, ErrInsufficient
	}
	// savepoint: нарушение уникальности не должно ломать внешнюю транзакцию
	sp, err := tx.Begin(ctx)
	if err != nil {
		return 0, err
	}
	_, err = sp.Exec(ctx, `
		INSERT INTO token_ledger (user_id, op, amount, balance_after, reference_type, reference_id, reason, actor_id)
		VALUES ($1, $2, $3, $4, NULLIF($5, ''), $6, NULLIF($7, ''), $8)`,
		e.UserID, e.Op, e.Amount, newBal, e.ReferenceType, e.ReferenceID, e.Reason, e.ActorID)
	if err != nil {
		_ = sp.Rollback(ctx)
		if db.IsUniqueViolation(err, "") {
			return bal, ErrDuplicate
		}
		return 0, err
	}
	if err := sp.Commit(ctx); err != nil {
		return 0, err
	}
	if _, err := tx.Exec(ctx, `UPDATE users SET token_balance = $2 WHERE id = $1`, e.UserID, newBal); err != nil {
		return 0, err
	}
	return newBal, nil
}

// Setting читает числовую настройку платформы (generation_cost, welcome_bonus).
func Setting(ctx context.Context, q interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, key string) (int64, error) {
	var v int64
	err := q.QueryRow(ctx, `SELECT (value #>> '{}')::bigint FROM platform_settings WHERE key = $1`, key).Scan(&v)
	return v, db.NotFound(err)
}
