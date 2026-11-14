package auth

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"math/big"
	"time"

	"github.com/redis/go-redis/v9"
)

// SMSSender отправляет код. В prod — реальный провайдер (Eskiz/Playmobile и т. п.).
type SMSSender interface {
	SendOTP(ctx context.Context, phone, code string) error
}

// LogSender пишет код в лог. ТОЛЬКО для dev/test — main запрещает его в prod.
type LogSender struct{ Log *slog.Logger }

func (s LogSender) SendOTP(_ context.Context, phone, code string) error {
	s.Log.Warn("DEV OTP (never in prod)", slog.String("phone_tail", tail(phone)), slog.String("code", code))
	return nil
}

// SendText — SMS-уведомление в лог (dev/test).
func (s LogSender) SendText(_ context.Context, phone, text string) error {
	s.Log.Info("DEV SMS (never in prod)", slog.String("phone_tail", tail(phone)), slog.String("text", text))
	return nil
}

func tail(p string) string {
	if len(p) <= 4 {
		return p
	}
	return "…" + p[len(p)-4:]
}

var (
	ErrOTPCooldown = errors.New("otp: resend cooldown")
	ErrOTPInvalid  = errors.New("otp: invalid or expired")
)

type OTP struct {
	rdb      *redis.Client
	pepper   []byte
	ttl      time.Duration
	cooldown time.Duration
	maxTries int
}

func NewOTP(rdb *redis.Client, pepper []byte, ttl, cooldown time.Duration, maxTries int) *OTP {
	return &OTP{rdb: rdb, pepper: pepper, ttl: ttl, cooldown: cooldown, maxTries: maxTries}
}

func (o *OTP) hash(phone, code string) string {
	m := hmac.New(sha256.New, o.pepper)
	m.Write([]byte(phone + "|" + code))
	return hex.EncodeToString(m.Sum(nil))
}

// Issue генерирует 6-значный код, хранит только HMAC и ставит cooldown на повторную отправку.
func (o *OTP) Issue(ctx context.Context, phone string) (string, error) {
	ok, err := o.rdb.SetNX(ctx, "otp:cd:"+phone, 1, o.cooldown).Result()
	if err != nil {
		return "", err
	}
	if !ok {
		return "", ErrOTPCooldown
	}
	n, err := rand.Int(rand.Reader, big.NewInt(1_000_000))
	if err != nil {
		return "", err
	}
	code := fmt.Sprintf("%06d", n.Int64())
	key := "otp:" + phone
	pipe := o.rdb.TxPipeline()
	pipe.Del(ctx, key)
	pipe.HSet(ctx, key, "h", o.hash(phone, code), "t", 0)
	pipe.Expire(ctx, key, o.ttl)
	if _, err := pipe.Exec(ctx); err != nil {
		return "", err
	}
	return code, nil
}

// Cancel снимает выданный код и cooldown: код не дошёл до клиента (сбой отправки),
// поэтому повторный запрос не должен упираться в «код уже отправлен».
func (o *OTP) Cancel(ctx context.Context, phone string) {
	_ = o.rdb.Del(ctx, "otp:"+phone, "otp:cd:"+phone).Err()
}

// попытка засчитывается атомарно и до сравнения
var otpAttempt = redis.NewScript(`
if redis.call('EXISTS', KEYS[1]) == 0 then return false end
local t = redis.call('HINCRBY', KEYS[1], 't', 1)
return {t, redis.call('HGET', KEYS[1], 'h')}
`)

// Verify проверяет код. После maxTries попыток код сгорает. Успех — одноразовый.
func (o *OTP) Verify(ctx context.Context, phone, code string) error {
	key := "otp:" + phone
	res, err := otpAttempt.Run(ctx, o.rdb, []string{key}).Slice()
	if errors.Is(err, redis.Nil) {
		return ErrOTPInvalid
	}
	if err != nil {
		return err
	}
	tries, _ := res[0].(int64)
	stored, _ := res[1].(string)
	if int(tries) > o.maxTries {
		o.rdb.Del(ctx, key)
		return ErrOTPInvalid
	}
	if subtle.ConstantTimeCompare([]byte(stored), []byte(o.hash(phone, code))) != 1 {
		return ErrOTPInvalid
	}
	// Del возвращает 1 только одному из параллельных запросов — защита от повторного использования.
	if n, err := o.rdb.Del(ctx, key).Result(); err != nil || n != 1 {
		return ErrOTPInvalid
	}
	return nil
}
