// Package payments — покупка пакетов AI-токенов через Click и Payme.
//
// Правила (не нарушать при доработке):
//   - Платёж создаёт клиент (POST /v1/payments): сумма и число токенов копируются из
//     тарифа в момент создания, вебхуки сверяют сумму только с платежом.
//   - Любой вебхук сначала блокирует строку платежа (SELECT … FOR UPDATE), затем
//     транзакцию провайдера: все операции по одному платежу строго последовательны.
//   - Зачисление токенов — только в markPaid, в той же транзакции, что и смена статуса;
//     двойное зачисление исключают статус pending→paid, уникальный индекс
//     token_ledger_ref_once (purchase, payment, id) и индекс «одна живая транзакция
//     на платёж» (payment_transactions_one_active).
//   - Секреты провайдеров сравниваются за постоянное время; пустой секрет = провайдер выключен.
package payments

import (
	"context"
	"errors"
	"net/netip"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/boutique-market/backend/internal/platform/db"
	"github.com/boutique-market/backend/internal/tokens"
)

const (
	ProviderClick = "click"
	ProviderPayme = "payme"

	StatusPending   = "pending"
	StatusPaid      = "paid"
	StatusDeclined  = "declined"
	StatusCancelled = "cancelled"

	TxnCreated         = 1
	TxnPerformed       = 2
	TxnCancelled       = -1
	TxnCancelledRefund = -2
)

type payment struct {
	ID       uuid.UUID
	Number   int64
	UserID   uuid.UUID
	Provider string
	Amount   int64
	Tokens   int64
	Status   string
}

// lockPayment блокирует строку платежа до конца транзакции.
func lockPayment(ctx context.Context, tx pgx.Tx, id uuid.UUID) (*payment, error) {
	var p payment
	err := tx.QueryRow(ctx, `
		SELECT id, number, user_id, provider::text, amount, tokens, status::text
		FROM payments WHERE id = $1 FOR UPDATE`, id).
		Scan(&p.ID, &p.Number, &p.UserID, &p.Provider, &p.Amount, &p.Tokens, &p.Status)
	if err != nil {
		return nil, db.NotFound(err)
	}
	return &p, nil
}

type txn struct {
	ID           int64
	PaymentID    uuid.UUID
	ExternalID   string
	State        int
	Amount       int64
	ProviderTime int64
	CreatedAt    time.Time
	PerformedAt  *time.Time
	CancelledAt  *time.Time
	CancelReason *int
}

const txnCols = `id, payment_id, external_id, state, amount, COALESCE(provider_time, 0), created_at, performed_at, cancelled_at, cancel_reason`

func scanTxn(row pgx.Row) (*txn, error) {
	var t txn
	err := row.Scan(&t.ID, &t.PaymentID, &t.ExternalID, &t.State, &t.Amount, &t.ProviderTime,
		&t.CreatedAt, &t.PerformedAt, &t.CancelledAt, &t.CancelReason)
	if err != nil {
		return nil, db.NotFound(err)
	}
	return &t, nil
}

// markPaid проводит платёж и зачисляет токены. Платёж должен быть заблокирован и в статусе pending.
func markPaid(ctx context.Context, tx pgx.Tx, p *payment, providerTxnID string, at time.Time) error {
	tag, err := tx.Exec(ctx, `UPDATE payments SET status = 'paid', paid_at = $2, provider_txn_id = $3
		WHERE id = $1 AND status = 'pending'`, p.ID, at, providerTxnID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return errors.New("payments: payment is not pending")
	}
	_, err = tokens.Apply(ctx, tx, tokens.Entry{
		UserID: p.UserID, Op: tokens.OpPurchase, Amount: p.Tokens,
		ReferenceType: "payment", ReferenceID: &p.ID,
	})
	if err != nil {
		// ErrDuplicate здесь означал бы нарушение инварианта — откатываем всё.
		return err
	}
	// TODO(notify): notify.Send(ctx, tx, p.UserID, "payment_paid", map[string]any{"payment_id": p.ID, "tokens": p.Tokens})
	p.Status = StatusPaid
	return nil
}

// markFailed закрывает неоплаченный платёж (declined — отказ провайдера, cancelled — отмена/таймаут).
func markFailed(ctx context.Context, tx pgx.Tx, p *payment, status string) error {
	if p.Status != StatusPending {
		return nil
	}
	if _, err := tx.Exec(ctx, `UPDATE payments SET status = $2, cancelled_at = now() WHERE id = $1 AND status = 'pending'`,
		p.ID, status); err != nil {
		return err
	}
	// TODO(notify): notify.Send(ctx, tx, p.UserID, "payment_failed", map[string]any{"payment_id": p.ID})
	p.Status = status
	return nil
}

// ExpirePending закрывает брошенные платежи (нет живой транзакции провайдера дольше maxAge).
// Вызывается janitor-ом.
func ExpirePending(ctx context.Context, q interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
}, maxAge time.Duration) (int64, error) {
	tag, err := q.Exec(ctx, `
		UPDATE payments p SET status = 'cancelled', cancelled_at = now()
		WHERE p.status = 'pending' AND p.created_at < now() - make_interval(secs => $1)
		  AND NOT EXISTS (SELECT 1 FROM payment_transactions t WHERE t.payment_id = p.id AND t.state = 1)`,
		maxAge.Seconds())
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

// ipAllow — необязательный список IP/подсетей провайдера. Пустой список пропускает всех.
type ipAllow []netip.Prefix

func parseAllow(list []string) (ipAllow, error) {
	var out ipAllow
	for _, s := range list {
		s = strings.TrimSpace(s)
		if !strings.Contains(s, "/") {
			a, err := netip.ParseAddr(s)
			if err != nil {
				return nil, err
			}
			out = append(out, netip.PrefixFrom(a, a.BitLen()))
			continue
		}
		p, err := netip.ParsePrefix(s)
		if err != nil {
			return nil, err
		}
		out = append(out, p.Masked())
	}
	return out, nil
}

func (a ipAllow) allows(ip string) bool {
	if len(a) == 0 {
		return true
	}
	addr, err := netip.ParseAddr(ip)
	if err != nil {
		return false
	}
	addr = addr.Unmap()
	for _, p := range a {
		if p.Contains(addr) {
			return true
		}
	}
	return false
}
