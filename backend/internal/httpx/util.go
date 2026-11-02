package httpx

import (
	"net/http"
	"reflect"
	"regexp"
	"strconv"
)

type reflectStructField = reflect.StructField

// e164 — телефон в международном формате, например +998901234567.
var e164 = regexp.MustCompile(`^\+[1-9][0-9]{7,14}$`)

func IsE164(s string) bool { return e164.MatchString(s) }

// Limit читает ?limit= с ограничением сверху.
func Limit(r *http.Request, def, max int) int {
	n, err := strconv.Atoi(r.URL.Query().Get("limit"))
	if err != nil || n <= 0 {
		return def
	}
	if n > max {
		return max
	}
	return n
}
