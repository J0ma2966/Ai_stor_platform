package promotions

import (
	"testing"
	"time"
)

func TestPeriod(t *testing.T) {
	now := time.Date(2026, 11, 6, 12, 0, 0, 0, time.UTC)
	at := func(h int) *time.Time { v := now.Add(time.Duration(h) * time.Hour); return &v }

	s, e, err := period(periodReq{}, 7, now)
	if err != nil || !s.Equal(now) || e.Sub(s) != 7*24*time.Hour {
		t.Fatalf("default: %v %v %v", s, e, err)
	}
	if s, e, err = period(periodReq{StartsAt: at(24)}, 3, now); err != nil || e.Sub(s) != 72*time.Hour {
		t.Fatalf("start only: %v %v %v", s, e, err)
	}
	for name, req := range map[string]periodReq{
		"end before start": {StartsAt: at(48), EndsAt: at(24)},
		"end in past":      {StartsAt: at(-20), EndsAt: at(-1)},
		"start too old":    {StartsAt: at(-25)},
		"too long":         {EndsAt: at(366 * 24)},
	} {
		if _, _, err := period(req, 7, now); err == nil {
			t.Errorf("%s accepted", name)
		}
	}
}

func TestCursorRoundTrip(t *testing.T) {
	ts := time.Date(2026, 11, 6, 12, 0, 0, 123456000, time.UTC)
	id := [16]byte{1, 2, 3}
	gt, gid, err := decodeCursor(encodeCursor(ts, id))
	if err != nil || !gt.Equal(ts) || gid != id {
		t.Fatalf("cursor: %v %v %v", gt, gid, err)
	}
	if _, _, err := decodeCursor("!!"); err == nil {
		t.Fatal("bad cursor accepted")
	}
}
