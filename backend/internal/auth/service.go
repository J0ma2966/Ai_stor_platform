package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"net/netip"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/boutique-market/backend/internal/platform/db"
)

var (
	ErrBadCredentials = errors.New("bad credentials")
	ErrAccountBlocked = errors.New("account blocked")
	ErrStoreInactive  = errors.New("store not active")
	ErrRefreshInvalid = errors.New("refresh token invalid")
)

type Service struct {
	DB         *pgxpool.Pool
	Signer     *Signer
	OTP        *OTP
	SMS        SMSSender
	RefreshTTL time.Duration
}

type Session struct {
	AccessToken  string    `json:"access_token"`
	ExpiresAt    time.Time `json:"expires_at"`
	refreshToken string
	refreshExp   time.Time
	Principal    Principal `json:"-"`
	IsNew        bool      `json:"is_new_user"`
}

type ClientMeta struct {
	UserAgent string
	IP        string
}

func (s *Service) RequestOTP(ctx context.Context, phone string) error {
	code, err := s.OTP.Issue(ctx, phone)
	if err != nil {
		return err
	}
	return s.SMS.SendOTP(ctx, phone, code)
}

// VerifyOTP: вход или регистрация клиента по телефону.
// Новый пользователь создаётся без пола; бонус начисляется после завершения регистрации (users.CompleteRegistration).
func (s *Service) VerifyOTP(ctx context.Context, phone, code string, meta ClientMeta) (*Session, error) {
	if err := s.OTP.Verify(ctx, phone, code); err != nil {
		return nil, err
	}
	p, isNew, err := s.clientByPhone(ctx, phone)
	if err != nil {
		return nil, err
	}
	sess, err := s.newSession(ctx, p, uuid.New(), meta)
	if err != nil {
		return nil, err
	}
	sess.IsNew = isNew
	return sess, nil
}

// clientByPhone находит клиента по уже подтверждённому телефону или создаёт нового.
func (s *Service) clientByPhone(ctx context.Context, phone string) (Principal, bool, error) {
	var (
		p      = Principal{Role: RoleClient}
		status string
		isNew  bool
	)
	err := db.Tx(ctx, s.DB, func(tx pgx.Tx) error {
		err := tx.QueryRow(ctx, `
			INSERT INTO users (role, phone) VALUES ('client', $1)
			ON CONFLICT (phone) DO NOTHING
			RETURNING id`, phone).Scan(&p.UserID)
		if err == nil {
			isNew, status = true, "active"
			return nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		var role Role
		if err := tx.QueryRow(ctx, `SELECT id, role, status FROM users WHERE phone = $1`, phone).Scan(&p.UserID, &role, &status); err != nil {
			return err
		}
		if role != RoleClient {
			return ErrBadCredentials // телефон сотрудника не даёт клиентский вход
		}
		return nil
	})
	if err != nil {
		return p, false, err
	}
	if status != "active" {
		return p, false, ErrAccountBlocked
	}
	return p, isNew, nil
}

// Login — вход магазина или администратора по логину и паролю.
func (s *Service) Login(ctx context.Context, login, password string, meta ClientMeta) (*Session, error) {
	var (
		p           Principal
		hash        string
		status      string
		storeID     *uuid.UUID
		storeStatus *string
	)
	err := s.DB.QueryRow(ctx, `
		SELECT u.id, u.role, u.password_hash, u.status, u.store_id, s.status
		FROM users u LEFT JOIN stores s ON s.id = u.store_id
		WHERE u.login = $1 AND u.role IN ('store','admin')`, login).
		Scan(&p.UserID, &p.Role, &hash, &status, &storeID, &storeStatus)
	if errors.Is(err, pgx.ErrNoRows) {
		_, _ = VerifyPassword(password, dummyHash)
		return nil, ErrBadCredentials
	}
	if err != nil {
		return nil, err
	}
	ok, err := VerifyPassword(password, hash)
	if err != nil || !ok {
		return nil, ErrBadCredentials
	}
	if status != "active" {
		return nil, ErrAccountBlocked
	}
	if p.Role == RoleStore {
		if storeStatus == nil || *storeStatus != "active" {
			return nil, ErrStoreInactive
		}
		p.StoreID = *storeID
	}
	return s.newSession(ctx, p, uuid.New(), meta)
}

// Refresh ротирует refresh-токен. Повторное использование уже использованного токена
// означает кражу: отзывается вся семья (все устройства этой цепочки).
func (s *Service) Refresh(ctx context.Context, raw string, meta ClientMeta) (*Session, error) {
	h := hashToken(raw)
	var (
		id, family uuid.UUID
		p          Principal
		usedAt     *time.Time
		revokedAt  *time.Time
		expiresAt  time.Time
		status     string
		storeID    *uuid.UUID
		storeSt    *string
	)
	err := s.DB.QueryRow(ctx, `
		SELECT rt.id, rt.family_id, rt.used_at, rt.revoked_at, rt.expires_at,
		       u.id, u.role, u.status, u.store_id, st.status
		FROM refresh_tokens rt
		JOIN users u ON u.id = rt.user_id
		LEFT JOIN stores st ON st.id = u.store_id
		WHERE rt.token_hash = $1`, h).
		Scan(&id, &family, &usedAt, &revokedAt, &expiresAt, &p.UserID, &p.Role, &status, &storeID, &storeSt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrRefreshInvalid
	}
	if err != nil {
		return nil, err
	}
	if usedAt != nil {
		_, _ = s.DB.Exec(ctx, `UPDATE refresh_tokens SET revoked_at = now() WHERE family_id = $1 AND revoked_at IS NULL`, family)
		return nil, ErrRefreshInvalid
	}
	if revokedAt != nil || time.Now().After(expiresAt) {
		return nil, ErrRefreshInvalid
	}
	if status != "active" {
		return nil, ErrAccountBlocked
	}
	if p.Role == RoleStore {
		if storeSt == nil || *storeSt != "active" {
			return nil, ErrStoreInactive
		}
		p.StoreID = *storeID
	}
	// Помечаем использованным атомарно: при гонке выиграет один запрос.
	tag, err := s.DB.Exec(ctx, `UPDATE refresh_tokens SET used_at = now() WHERE id = $1 AND used_at IS NULL`, id)
	if err != nil {
		return nil, err
	}
	if tag.RowsAffected() != 1 {
		return nil, ErrRefreshInvalid
	}
	return s.newSession(ctx, p, family, meta)
}

// Logout отзывает всю семью токена (текущее устройство).
func (s *Service) Logout(ctx context.Context, raw string) error {
	_, err := s.DB.Exec(ctx, `
		UPDATE refresh_tokens SET revoked_at = now()
		WHERE family_id = (SELECT family_id FROM refresh_tokens WHERE token_hash = $1) AND revoked_at IS NULL`, hashToken(raw))
	return err
}

func (s *Service) newSession(ctx context.Context, p Principal, family uuid.UUID, meta ClientMeta) (*Session, error) {
	access, exp, err := s.Signer.Sign(p)
	if err != nil {
		return nil, err
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return nil, err
	}
	rt := base64.RawURLEncoding.EncodeToString(raw)
	rexp := time.Now().Add(s.RefreshTTL)
	var ip *netip.Addr
	if a, err := netip.ParseAddr(meta.IP); err == nil {
		ip = &a
	}
	ua := meta.UserAgent
	if len(ua) > 300 {
		ua = ua[:300]
	}
	if _, err := s.DB.Exec(ctx, `
		INSERT INTO refresh_tokens (user_id, family_id, token_hash, expires_at, user_agent, ip)
		VALUES ($1, $2, $3, $4, $5, $6)`, p.UserID, family, hashToken(rt), rexp, ua, ip); err != nil {
		return nil, err
	}
	if _, err := s.DB.Exec(ctx, `UPDATE users SET last_login_at = now() WHERE id = $1`, p.UserID); err != nil {
		return nil, err
	}
	return &Session{AccessToken: access, ExpiresAt: exp, refreshToken: rt, refreshExp: rexp, Principal: p}, nil
}

func hashToken(raw string) []byte {
	h := sha256.Sum256([]byte(raw))
	return h[:]
}
