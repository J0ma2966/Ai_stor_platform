package auth

import (
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestVerifyTelegramInitData(t *testing.T) {
	const token = "123:ABC"
	now := time.Unix(1_800_000_000, 0)
	base := func(at time.Time) url.Values {
		return url.Values{
			"auth_date": {strconv.FormatInt(at.Unix(), 10)},
			"query_id":  {"AAH"},
			"user":      {`{"id":777001,"first_name":"Ali","language_code":"ru"}`},
		}
	}
	good := SignTelegramInitData(base(now.Add(-time.Minute)), token)
	if id, err := VerifyTelegramInitData(good, token, now); err != nil || id != 777001 {
		t.Fatalf("good: %d %v", id, err)
	}
	cases := map[string]string{
		"wrong token": SignTelegramInitData(base(now), "999:XYZ"),
		"too old":     SignTelegramInitData(base(now.Add(-25*time.Hour)), token),
		"future":      SignTelegramInitData(base(now.Add(time.Hour)), token),
		"tampered":    strings.Replace(good, "777001", "777002", 1),
		"no hash":     base(now).Encode(),
		"empty":       "",
	}
	for name, data := range cases {
		if _, err := VerifyTelegramInitData(data, token, now); err != ErrTelegramInitData {
			t.Errorf("%s: want ErrTelegramInitData, got %v", name, err)
		}
	}
	if _, err := VerifyTelegramInitData(good, "", now); err != ErrTelegramInitData {
		t.Error("empty bot token must fail")
	}
}
