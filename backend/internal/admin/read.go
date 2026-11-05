package admin

// Чтение для веб-панели суперадмина: статистика, списки магазинов/пользователей,
// журнал аудита. Все списки — курсорная пагинация (см. docs/API.md).

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/boutique-market/backend/internal/httpx"
	"github.com/boutique-market/backend/internal/platform/db"
)

// ---------- общие помощники ----------

// where собирает условия с позиционными параметрами $1..$n.
type where struct {
	conds []string
	args  []any
}

// add добавляет условие; "?" в cond заменяется на номер очередного параметра.
func (w *where) add(cond string, args ...any) {
	for _, a := range args {
		w.args = append(w.args, a)
		cond = strings.Replace(cond, "?", "$"+strconv.Itoa(len(w.args)), 1)
	}
	w.conds = append(w.conds, cond)
}

func (w *where) sql() string {
	if len(w.conds) == 0 {
		return "true"
	}
	return strings.Join(w.conds, " AND ")
}

// likePattern экранирует спецсимволы LIKE и возвращает шаблон «содержит».
func likePattern(q string) string {
	r := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return "%" + r.Replace(q) + "%"
}

func encodeTimeCursor(t time.Time, id uuid.UUID) string {
	return base64.RawURLEncoding.EncodeToString([]byte(t.UTC().Format(time.RFC3339Nano) + "|" + id.String()))
}

func decodeTimeCursor(c string) (time.Time, uuid.UUID, error) {
	raw, err := base64.RawURLEncoding.DecodeString(c)
	if err != nil {
		return time.Time{}, uuid.Nil, err
	}
	ts, id, ok := strings.Cut(string(raw), "|")
	if !ok {
		return time.Time{}, uuid.Nil, errors.New("bad cursor")
	}
	t, err := time.Parse(time.RFC3339Nano, ts)
	if err != nil {
		return time.Time{}, uuid.Nil, err
	}
	u, err := uuid.Parse(id)
	return t, u, err
}

var errBadCursor = httpx.NewError(http.StatusBadRequest, "bad_cursor", "invalid cursor")

func badFilter(msg string) error { return httpx.NewError(http.StatusBadRequest, "bad_filter", msg) }

func oneOf(v string, allowed ...string) bool {
	for _, a := range allowed {
		if v == a {
			return true
		}
	}
	return false
}

// ---------- статистика ----------

type Stats struct {
	Stores struct {
		Total             int `json:"total"`
		Active            int `json:"active"`
		PendingActivation int `json:"pending_activation"`
		Blocked           int `json:"blocked"`
		Archived          int `json:"archived"`
	} `json:"stores"`
	Users struct {
		Clients     int `json:"clients"`
		ClientsNew7 int `json:"clients_new_7d"`
		Blocked     int `json:"blocked"`
	} `json:"users"`
	Orders struct {
		Total    int            `json:"total"`
		Today    int            `json:"today"`
		Last7    int            `json:"last_7d"`
		ByStatus map[string]int `json:"by_status"`
		// Сумма доставленных заказов за 30 дней, в тийинах.
		DeliveredSum30 int64 `json:"delivered_sum_30d"`
	} `json:"orders"`
	Products struct {
		Published int `json:"published"`
		Total     int `json:"total"`
	} `json:"products"`
	Tokens struct {
		Issued30 int64 `json:"issued_30d"`
		Spent30  int64 `json:"spent_30d"`
	} `json:"tokens"`
}

func (h *Handlers) stats(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var s Stats
	// «Сегодня» — по времени Ташкента (UTC+5), как видит админ.
	err := h.DB.QueryRow(ctx, `
		SELECT
		  (SELECT count(*) FROM stores),
		  (SELECT count(*) FROM stores WHERE status = 'active'),
		  (SELECT count(*) FROM stores WHERE status = 'pending_activation'),
		  (SELECT count(*) FROM stores WHERE status = 'blocked'),
		  (SELECT count(*) FROM stores WHERE status = 'archived'),
		  (SELECT count(*) FROM users WHERE role = 'client'),
		  (SELECT count(*) FROM users WHERE role = 'client' AND created_at >= now() - interval '7 days'),
		  (SELECT count(*) FROM users WHERE status = 'blocked'),
		  (SELECT count(*) FROM orders),
		  (SELECT count(*) FROM orders WHERE created_at >= date_trunc('day', now() AT TIME ZONE 'Asia/Tashkent') AT TIME ZONE 'Asia/Tashkent'),
		  (SELECT count(*) FROM orders WHERE created_at >= now() - interval '7 days'),
		  (SELECT COALESCE(sum(items_total), 0) FROM orders WHERE status = 'delivered' AND updated_at >= now() - interval '30 days')::bigint,
		  (SELECT count(*) FROM products WHERE status = 'published'),
		  (SELECT count(*) FROM products),
		  (SELECT COALESCE(sum(amount) FILTER (WHERE amount > 0), 0) FROM token_ledger WHERE created_at >= now() - interval '30 days')::bigint,
		  (SELECT COALESCE(-sum(amount) FILTER (WHERE amount < 0), 0) FROM token_ledger WHERE created_at >= now() - interval '30 days')::bigint`).
		Scan(&s.Stores.Total, &s.Stores.Active, &s.Stores.PendingActivation, &s.Stores.Blocked, &s.Stores.Archived,
			&s.Users.Clients, &s.Users.ClientsNew7, &s.Users.Blocked,
			&s.Orders.Total, &s.Orders.Today, &s.Orders.Last7, &s.Orders.DeliveredSum30,
			&s.Products.Published, &s.Products.Total,
			&s.Tokens.Issued30, &s.Tokens.Spent30)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	s.Orders.ByStatus = map[string]int{}
	for _, st := range []string{"pending", "confirmed", "rejected", "assembling", "in_delivery", "delivered", "cancelled"} {
		s.Orders.ByStatus[st] = 0
	}
	rows, err := h.DB.Query(ctx, `SELECT status::text, count(*) FROM orders GROUP BY status`)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	defer rows.Close()
	for rows.Next() {
		var st string
		var n int
		if err := rows.Scan(&st, &n); err != nil {
			httpx.Fail(w, r, err)
			return
		}
		s.Orders.ByStatus[st] = n
	}
	if err := rows.Err(); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, s)
}

// ---------- магазины ----------

type StoreRow struct {
	ID            uuid.UUID  `json:"id"`
	Slug          string     `json:"slug"`
	Name          string     `json:"name"`
	Phone         *string    `json:"phone"`
	Status        string     `json:"status"`
	Login         *string    `json:"login"`
	UserID        *uuid.UUID `json:"user_id"`
	ProductsCount int        `json:"products_count"`
	OrdersCount   int        `json:"orders_count"`
	ActivatedAt   *time.Time `json:"activated_at"`
	CreatedAt     time.Time  `json:"created_at"`
}

const storeCols = `s.id, s.slug, s.name, s.phone, s.status::text, u.login, u.id,
	(SELECT count(*) FROM products p WHERE p.store_id = s.id)::int,
	(SELECT count(*) FROM orders o WHERE o.store_id = s.id)::int,
	s.activated_at, s.created_at`

const storeDetailSQL = `SELECT ` + storeCols + `, s.description, s.address, s.contacts, u.status::text, u.last_login_at
	FROM stores s LEFT JOIN users u ON u.store_id = s.id
	WHERE s.id = $1`

func scanStore(row pgx.Row, s *StoreRow) error {
	return row.Scan(&s.ID, &s.Slug, &s.Name, &s.Phone, &s.Status, &s.Login, &s.UserID,
		&s.ProductsCount, &s.OrdersCount, &s.ActivatedAt, &s.CreatedAt)
}

func (h *Handlers) listStores(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	var wh where
	if st := q.Get("status"); st != "" {
		if !oneOf(st, "pending_activation", "active", "blocked", "archived") {
			httpx.Fail(w, r, badFilter("unknown status"))
			return
		}
		wh.add("s.status::text = ?", st)
	}
	if s := strings.TrimSpace(q.Get("q")); s != "" {
		if len(s) > 100 {
			httpx.Fail(w, r, badFilter("query too long"))
			return
		}
		p := likePattern(s)
		wh.add("(s.name ILIKE ? OR s.slug ILIKE ? OR u.login ILIKE ?)", p, p, p)
	}
	if c := q.Get("cursor"); c != "" {
		t, id, err := decodeTimeCursor(c)
		if err != nil {
			httpx.Fail(w, r, errBadCursor)
			return
		}
		wh.add("(s.created_at, s.id) < (?, ?)", t, id)
	}
	limit := httpx.Limit(r, 20, 100)
	rows, err := h.DB.Query(r.Context(), `
		SELECT `+storeCols+`
		FROM stores s LEFT JOIN users u ON u.store_id = s.id
		WHERE `+wh.sql()+`
		ORDER BY s.created_at DESC, s.id DESC LIMIT `+strconv.Itoa(limit+1), wh.args...)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	defer rows.Close()
	items, err := collect(rows, scanStore)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	next := ""
	if len(items) > limit {
		items = items[:limit]
		next = encodeTimeCursor(items[limit-1].CreatedAt, items[limit-1].ID)
	}
	httpx.List(w, items, next)
}

type StoreDetail struct {
	StoreRow
	Description string          `json:"description"`
	Address     *string         `json:"address"`
	Contacts    json.RawMessage `json:"contacts"`
	UserStatus  *string         `json:"user_status"`
	LastLoginAt *time.Time      `json:"last_login_at"`
	History     []AuditEntry    `json:"history"`
}

func (h *Handlers) getStore(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httpx.Fail(w, r, httpx.ErrNotFound)
		return
	}
	var d StoreDetail
	row := h.DB.QueryRow(r.Context(), storeDetailSQL, id)
	err = row.Scan(&d.ID, &d.Slug, &d.Name, &d.Phone, &d.Status, &d.Login, &d.UserID,
		&d.ProductsCount, &d.OrdersCount, &d.ActivatedAt, &d.CreatedAt,
		&d.Description, &d.Address, &d.Contacts, &d.UserStatus, &d.LastLoginAt)
	if errors.Is(db.NotFound(err), db.ErrNotFound) {
		httpx.Fail(w, r, httpx.ErrNotFound)
		return
	}
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	rows, err := h.DB.Query(r.Context(), `
		SELECT `+auditCols+` FROM audit_log a LEFT JOIN users u ON u.id = a.actor_id
		WHERE a.entity_type = 'store' AND a.entity_id = $1
		ORDER BY a.created_at DESC, a.id DESC LIMIT 20`, id.String())
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	defer rows.Close()
	if d.History, err = collect(rows, scanAudit); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, d)
}

// ---------- пользователи ----------

type UserRow struct {
	ID           uuid.UUID  `json:"id"`
	Role         string     `json:"role"`
	Status       string     `json:"status"`
	Phone        *string    `json:"phone"`
	Login        *string    `json:"login"`
	StoreID      *uuid.UUID `json:"store_id"`
	StoreName    *string    `json:"store_name"`
	Gender       *string    `json:"gender"`
	Lang         string     `json:"lang"`
	TokenBalance int64      `json:"token_balance"`
	OrdersCount  int        `json:"orders_count"`
	Registered   bool       `json:"registration_complete"`
	LastLoginAt  *time.Time `json:"last_login_at"`
	CreatedAt    time.Time  `json:"created_at"`
}

const userCols = `u.id, u.role::text, u.status::text, u.phone, u.login, u.store_id, s.name, u.gender::text, u.lang::text,
	u.token_balance, (SELECT count(*) FROM orders o WHERE o.user_id = u.id)::int,
	(u.role <> 'client' OR (u.gender IS NOT NULL AND u.pd_consent_at IS NOT NULL)),
	u.last_login_at, u.created_at`

const userDetailSQL = `SELECT ` + userCols + ` FROM users u LEFT JOIN stores s ON s.id = u.store_id WHERE u.id = $1`

func scanUser(row pgx.Row, u *UserRow) error {
	return row.Scan(&u.ID, &u.Role, &u.Status, &u.Phone, &u.Login, &u.StoreID, &u.StoreName, &u.Gender, &u.Lang,
		&u.TokenBalance, &u.OrdersCount, &u.Registered, &u.LastLoginAt, &u.CreatedAt)
}

func (h *Handlers) listUsers(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	var wh where
	if role := q.Get("role"); role != "" {
		if !oneOf(role, "client", "store", "admin") {
			httpx.Fail(w, r, badFilter("unknown role"))
			return
		}
		wh.add("u.role::text = ?", role)
	}
	if st := q.Get("status"); st != "" {
		if !oneOf(st, "active", "blocked") {
			httpx.Fail(w, r, badFilter("unknown status"))
			return
		}
		wh.add("u.status::text = ?", st)
	}
	if s := strings.TrimSpace(q.Get("q")); s != "" {
		if len(s) > 100 {
			httpx.Fail(w, r, badFilter("query too long"))
			return
		}
		p := likePattern(s)
		wh.add("(u.phone LIKE ? OR u.login ILIKE ? OR s.name ILIKE ?)", p, p, p)
	}
	if c := q.Get("cursor"); c != "" {
		t, id, err := decodeTimeCursor(c)
		if err != nil {
			httpx.Fail(w, r, errBadCursor)
			return
		}
		wh.add("(u.created_at, u.id) < (?, ?)", t, id)
	}
	limit := httpx.Limit(r, 20, 100)
	rows, err := h.DB.Query(r.Context(), `
		SELECT `+userCols+`
		FROM users u LEFT JOIN stores s ON s.id = u.store_id
		WHERE `+wh.sql()+`
		ORDER BY u.created_at DESC, u.id DESC LIMIT `+strconv.Itoa(limit+1), wh.args...)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	defer rows.Close()
	items, err := collect(rows, scanUser)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	next := ""
	if len(items) > limit {
		items = items[:limit]
		next = encodeTimeCursor(items[limit-1].CreatedAt, items[limit-1].ID)
	}
	httpx.List(w, items, next)
}

type LedgerEntry struct {
	ID           int64     `json:"id"`
	Op           string    `json:"op"`
	Amount       int64     `json:"amount"`
	BalanceAfter int64     `json:"balance_after"`
	Reason       *string   `json:"reason"`
	CreatedAt    time.Time `json:"created_at"`
}

type UserDetail struct {
	UserRow
	Ledger  []LedgerEntry `json:"ledger"`
	History []AuditEntry  `json:"history"`
}

func (h *Handlers) getUser(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httpx.Fail(w, r, httpx.ErrNotFound)
		return
	}
	var d UserDetail
	err = scanUser(h.DB.QueryRow(r.Context(), userDetailSQL, id), &d.UserRow)
	if errors.Is(db.NotFound(err), db.ErrNotFound) {
		httpx.Fail(w, r, httpx.ErrNotFound)
		return
	}
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	rows, err := h.DB.Query(r.Context(), `
		SELECT id, op::text, amount, balance_after, reason, created_at
		FROM token_ledger WHERE user_id = $1 ORDER BY id DESC LIMIT 30`, id)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if d.Ledger, err = collect(rows, func(row pgx.Row, e *LedgerEntry) error {
		return row.Scan(&e.ID, &e.Op, &e.Amount, &e.BalanceAfter, &e.Reason, &e.CreatedAt)
	}); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	rows, err = h.DB.Query(r.Context(), `
		SELECT `+auditCols+` FROM audit_log a LEFT JOIN users u ON u.id = a.actor_id
		WHERE a.entity_type = 'user' AND a.entity_id = $1
		ORDER BY a.created_at DESC, a.id DESC LIMIT 20`, id.String())
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	defer rows.Close()
	if d.History, err = collect(rows, scanAudit); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, d)
}

// ---------- журнал аудита ----------

type AuditEntry struct {
	ID         int64           `json:"id"`
	ActorID    *uuid.UUID      `json:"actor_id"`
	ActorRole  *string         `json:"actor_role"`
	ActorLogin *string         `json:"actor_login"`
	Action     string          `json:"action"`
	EntityType string          `json:"entity_type"`
	EntityID   *string         `json:"entity_id"`
	Data       json.RawMessage `json:"data"`
	IP         *string         `json:"ip"`
	RequestID  *string         `json:"request_id"`
	CreatedAt  time.Time       `json:"created_at"`
}

const auditCols = `a.id, a.actor_id, a.actor_role::text, COALESCE(u.login, u.phone), a.action, a.entity_type, a.entity_id,
	a.data, host(a.ip), a.request_id, a.created_at`

func scanAudit(row pgx.Row, e *AuditEntry) error {
	return row.Scan(&e.ID, &e.ActorID, &e.ActorRole, &e.ActorLogin, &e.Action, &e.EntityType, &e.EntityID,
		&e.Data, &e.IP, &e.RequestID, &e.CreatedAt)
}

func (h *Handlers) listAudit(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	var wh where
	if a := q.Get("action"); a != "" {
		if len(a) > 64 {
			httpx.Fail(w, r, badFilter("action too long"))
			return
		}
		// "store." — все действия над магазинами (префикс), иначе точное совпадение
		if strings.HasSuffix(a, ".") {
			wh.add("a.action LIKE ?", strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(a)+"%")
		} else {
			wh.add("a.action = ?", a)
		}
	}
	if et := q.Get("entity_type"); et != "" {
		if len(et) > 64 {
			httpx.Fail(w, r, badFilter("entity_type too long"))
			return
		}
		wh.add("a.entity_type = ?", et)
	}
	if eid := q.Get("entity_id"); eid != "" {
		if len(eid) > 64 {
			httpx.Fail(w, r, badFilter("entity_id too long"))
			return
		}
		wh.add("a.entity_id = ?", eid)
	}
	if aid := q.Get("actor_id"); aid != "" {
		u, err := uuid.Parse(aid)
		if err != nil {
			httpx.Fail(w, r, badFilter("bad actor_id"))
			return
		}
		wh.add("a.actor_id = ?", u)
	}
	if c := q.Get("cursor"); c != "" {
		n, err := strconv.ParseInt(c, 10, 64)
		if err != nil || n <= 0 {
			httpx.Fail(w, r, errBadCursor)
			return
		}
		wh.add("a.id < ?", n)
	}
	limit := httpx.Limit(r, 50, 200)
	rows, err := h.DB.Query(r.Context(), `
		SELECT `+auditCols+` FROM audit_log a LEFT JOIN users u ON u.id = a.actor_id
		WHERE `+wh.sql()+`
		ORDER BY a.id DESC LIMIT `+strconv.Itoa(limit+1), wh.args...)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	defer rows.Close()
	items, err := collect(rows, scanAudit)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	next := ""
	if len(items) > limit {
		items = items[:limit]
		next = strconv.FormatInt(items[limit-1].ID, 10)
	}
	httpx.List(w, items, next)
}

// collect читает все строки через scan; пустой результат — [] (а не null в JSON).
func collect[T any](rows pgx.Rows, scan func(pgx.Row, *T) error) ([]T, error) {
	defer rows.Close()
	out := []T{}
	for rows.Next() {
		var v T
		if err := scan(rows, &v); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
