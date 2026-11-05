package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"runtime"
	"strings"

	"golang.org/x/crypto/argon2"
)

// Параметры argon2id (OWASP: m=19 MiB, t=2, p=1 — минимум; берём с запасом).
const (
	argonTime    = 3
	argonMemory  = 64 * 1024
	argonThreads = 2
	argonKeyLen  = 32
)

var errBadHash = errors.New("invalid password hash")

// HashPassword возвращает строку в PHC-формате: $argon2id$v=19$m=..,t=..,p=..$salt$hash
func HashPassword(pw string) (string, error) {
	if len(pw) < 10 || len(pw) > 128 {
		return "", errors.New("password must be 10..128 characters")
	}
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	key := idKey([]byte(pw), salt, argonTime, argonMemory, argonThreads, argonKeyLen)
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s", argon2.Version, argonMemory, argonTime, argonThreads,
		base64.RawStdEncoding.EncodeToString(salt), base64.RawStdEncoding.EncodeToString(key)), nil
}

// VerifyPassword сравнивает за постоянное время.
func VerifyPassword(pw, encoded string) (bool, error) {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[1] != "argon2id" {
		return false, errBadHash
	}
	var m uint32
	var t uint32
	var p uint8
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &m, &t, &p); err != nil {
		return false, errBadHash
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return false, errBadHash
	}
	want, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return false, errBadHash
	}
	if len(want) < 16 || len(want) > 64 || m > 1<<21 || t > 16 {
		return false, errBadHash
	}
	got := idKey([]byte(pw), salt, t, m, p, uint32(len(want))) // #nosec G115 -- len checked above
	return subtle.ConstantTimeCompare(got, want) == 1, nil
}

// dummyHash используется, когда логин не найден, чтобы время ответа не выдавало существование аккаунта.
var dummyHash, _ = HashPassword("dummy-password-for-timing")

// argonSem ограничивает одновременные вычисления argon2id на инстанс: каждое берёт
// 64 МБ памяти, и сотня параллельных входов (ботнет обходит лимиты по IP) съела бы
// 6+ ГБ и уронила API по OOM. Лишние запросы ждут очереди, а не выделяют память.
var argonSem = make(chan struct{}, max(2, runtime.NumCPU()))

func idKey(pw, salt []byte, t, m uint32, p uint8, n uint32) []byte {
	argonSem <- struct{}{}
	defer func() { <-argonSem }()
	return argon2.IDKey(pw, salt, t, m, p, n)
}
