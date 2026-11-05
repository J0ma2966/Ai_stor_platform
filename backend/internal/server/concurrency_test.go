package server

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"

	"github.com/boutique-market/backend/internal/products"
)

// Тесты синхронизации: много покупателей и магазин действуют одновременно.
// Инварианты: ни одной 500, остаток не уходит в минус и не возвращается дважды,
// у каждого заказа ровно одна запись истории на каждый переход.

func buyers(t *testing.T, f *orderFixture, n int, prefix string) []*client {
	t.Helper()
	out := make([]*client, n)
	for i := range out {
		c := f.e.client()
		c.loginClient(fmt.Sprintf("+99890%03d%04d", 700+len(prefix), i))
		out[i] = c
	}
	return out
}

func setSizes(t *testing.T, f *orderFixture, prod string, sizes ...any) {
	t.Helper()
	var in []map[string]any
	for i := 0; i < len(sizes); i += 2 {
		in = append(in, map[string]any{"size": sizes[i], "stock": sizes[i+1]})
	}
	f.storeA.must(200, "PATCH", "/v1/store/products/"+prod, map[string]any{"sizes": in})
}

func sized(prod, size string, qty int) map[string]any {
	return map[string]any{"product_id": prod, "size": size, "qty": qty}
}

func TestConcurrency_LastItemSoldOnce(t *testing.T) {
	f := newOrderFixture(t)
	setSizes(t, f, f.prodA, "M", 1)
	cs := buyers(t, f, 12, "last")

	codes := make([]int, len(cs))
	var wg sync.WaitGroup
	for i, c := range cs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			codes[i] = c.doH("POST", "/v1/orders", checkoutBody(sized(f.prodA, "M", 1)), idem(fmt.Sprintf("last-item-%02d", i))).Status
		}()
	}
	wg.Wait()
	won := 0
	for i, s := range codes {
		switch s {
		case 201:
			won++
		case 409:
		default:
			t.Errorf("buyer %d: status %d (want 201 or 409)", i, s)
		}
	}
	if won != 1 {
		t.Fatalf("last item sold %d times", won)
	}
	if st, in := stockOf(t, f.e, f.prodA, "M"); st != 0 || in {
		t.Fatalf("stock=%d in_stock=%v", st, in)
	}
}

func TestConcurrency_StockNeverOversold(t *testing.T) {
	f := newOrderFixture(t)
	setSizes(t, f, f.prodA, "S", 3, "M", 4)
	cs := buyers(t, f, 20, "many")

	codes := make([]int, len(cs))
	var wg sync.WaitGroup
	for i, c := range cs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			// половина берёт S, половина M, часть — оба размера в разном порядке
			var body map[string]any
			switch i % 4 {
			case 0:
				body = checkoutBody(sized(f.prodA, "S", 1))
			case 1:
				body = checkoutBody(sized(f.prodA, "M", 1))
			case 2:
				body = checkoutBody(sized(f.prodA, "S", 1), sized(f.prodA, "M", 1))
			default:
				body = checkoutBody(sized(f.prodA, "M", 1), sized(f.prodA, "S", 1), item(f.prodB, 1))
			}
			codes[i] = c.doH("POST", "/v1/orders", body, idem(fmt.Sprintf("many-key-%02d", i))).Status
		}()
	}
	wg.Wait()
	for i, s := range codes {
		if s != 201 && s != 409 {
			t.Errorf("buyer %d: status %d", i, s)
		}
	}
	var soldS, soldM int
	_ = f.e.d.DB.QueryRow(context.Background(), `SELECT COALESCE(sum(qty) FILTER (WHERE size = 'S'), 0),
		COALESCE(sum(qty) FILTER (WHERE size = 'M'), 0) FROM order_items WHERE product_id = $1`, f.prodA).Scan(&soldS, &soldM)
	stS, _ := stockOf(t, f.e, f.prodA, "S")
	stM, _ := stockOf(t, f.e, f.prodA, "M")
	if soldS+stS != 3 || soldM+stM != 4 || stS < 0 || stM < 0 {
		t.Fatalf("S sold %d left %d; M sold %d left %d", soldS, stS, soldM, stM)
	}
}

// Клиент отменяет, а магазин в ту же секунду подтверждает/отклоняет: выигрывает
// кто-то один, остаток возвращается не больше одного раза.
func TestConcurrency_CancelVsStoreActions(t *testing.T) {
	f := newOrderFixture(t)
	setSizes(t, f, f.prodA, "S", 50, "M", 50)
	c := buyers(t, f, 1, "cancel")[0]
	ctx := context.Background()

	for round := 0; round < 8; round++ {
		r := c.mustH(201, "POST", "/v1/orders", checkoutBody(sized(f.prodA, "M", 2), sized(f.prodA, "S", 1)), idem(fmt.Sprintf("cancel-race-%02d", round)))
		id := r.data()["orders"].([]any)[0].(map[string]any)["id"].(string)

		storeBody := map[string]any{"status": "confirmed"}
		if round%2 == 1 {
			storeBody = map[string]any{"status": "rejected", "reason": "нет в наличии"}
		}
		var wg sync.WaitGroup
		var cancel1, cancel2, store int
		wg.Add(3)
		go func() { defer wg.Done(); cancel1 = c.do("POST", "/v1/orders/"+id+"/cancel", map[string]any{}).Status }()
		go func() { defer wg.Done(); cancel2 = c.do("POST", "/v1/orders/"+id+"/cancel", map[string]any{}).Status }()
		go func() {
			defer wg.Done()
			store = f.storeA.do("POST", "/v1/store/orders/"+id+"/status", storeBody).Status
		}()
		wg.Wait()
		for _, s := range []int{cancel1, cancel2, store} {
			if s != 200 && s != 409 {
				t.Fatalf("round %d: statuses cancel=%d,%d store=%d", round, cancel1, cancel2, store)
			}
		}
		var status string
		var closed int
		_ = f.e.d.DB.QueryRow(ctx, `SELECT status::text FROM orders WHERE id = $1`, id).Scan(&status)
		_ = f.e.d.DB.QueryRow(ctx, `SELECT count(*) FROM order_status_history WHERE order_id = $1
			AND to_status IN ('cancelled', 'rejected')`, id).Scan(&closed)
		if closed > 1 {
			t.Fatalf("round %d: order closed %d times", round, closed)
		}
		if status == "pending" {
			t.Fatalf("round %d: nobody won", round)
		}
	}
	// всё отменено/отклонено или подтверждено: остаток = 50 − неотменённые заказы
	var activeM, activeS int
	_ = f.e.d.DB.QueryRow(ctx, `SELECT COALESCE(sum(oi.qty) FILTER (WHERE oi.size = 'M'), 0), COALESCE(sum(oi.qty) FILTER (WHERE oi.size = 'S'), 0)
		FROM order_items oi JOIN orders o ON o.id = oi.order_id
		WHERE o.status NOT IN ('cancelled', 'rejected')`).Scan(&activeM, &activeS)
	stM, _ := stockOf(t, f.e, f.prodA, "M")
	stS, _ := stockOf(t, f.e, f.prodA, "S")
	if stM+activeM != 50 || stS+activeS != 50 {
		t.Fatalf("stock drift: M %d+%d, S %d+%d", stM, activeM, stS, activeS)
	}
}

// Одновременные отмены многоразмерных заказов и новые покупки тех же размеров
// не должны падать с deadlock (500).
func TestConcurrency_CancelAndCheckoutNoDeadlock(t *testing.T) {
	f := newOrderFixture(t)
	setSizes(t, f, f.prodA, "S", 1000, "M", 1000, "L", 1000)
	cs := buyers(t, f, 12, "dl")

	ids := make([]string, len(cs))
	for i, c := range cs {
		var body map[string]any
		if i%2 == 0 {
			body = checkoutBody(sized(f.prodA, "S", 1), sized(f.prodA, "M", 1), sized(f.prodA, "L", 1))
		} else {
			body = checkoutBody(sized(f.prodA, "L", 1), sized(f.prodA, "M", 1), sized(f.prodA, "S", 1))
		}
		ids[i] = c.mustH(201, "POST", "/v1/orders", body, idem(fmt.Sprintf("dl-key-a-%02d", i))).data()["orders"].([]any)[0].(map[string]any)["id"].(string)
	}
	var mu sync.Mutex
	var bad []string
	var wg sync.WaitGroup
	for i, c := range cs {
		wg.Add(2)
		go func() {
			defer wg.Done()
			if r := c.do("POST", "/v1/orders/"+ids[i]+"/cancel", map[string]any{}); r.Status != 200 {
				mu.Lock()
				bad = append(bad, fmt.Sprintf("cancel %d: %d %s", i, r.Status, r.Raw))
				mu.Unlock()
			}
		}()
		go func() {
			defer wg.Done()
			body := checkoutBody(sized(f.prodA, "L", 1), sized(f.prodA, "S", 1), sized(f.prodA, "M", 1))
			if r := c.doH("POST", "/v1/orders", body, idem(fmt.Sprintf("dl-key-b-%02d", i))); r.Status != 201 {
				mu.Lock()
				bad = append(bad, fmt.Sprintf("checkout %d: %d %s", i, r.Status, r.Raw))
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if len(bad) > 0 {
		t.Fatalf("failures:\n%v", bad)
	}
	for _, s := range []string{"S", "M", "L"} {
		if st, _ := stockOf(t, f.e, f.prodA, s); st != 1000-len(cs) {
			t.Fatalf("size %s stock %d", s, st)
		}
	}
}

// Сброс счётчиков просмотров/примерочной (ids в случайном порядке) идёт параллельно
// с оформлением заказов на те же товары: без единого порядка блокировок — deadlock.
func TestConcurrency_CounterFlushVsCheckout(t *testing.T) {
	f := newOrderFixture(t)
	ctx := context.Background()
	var catID uuid.UUID
	_ = f.e.d.DB.QueryRow(ctx, `SELECT id FROM categories LIMIT 1`).Scan(&catID)
	var ids []uuid.UUID
	for i := 0; i < 30; i++ {
		var id uuid.UUID
		if err := f.e.d.DB.QueryRow(ctx, `INSERT INTO products (store_id, category_id, sku, title, description, brand, price, in_stock, status)
			VALUES ($1, $2, $3, 'Т', 'D', 'B', 1000, true, 'published') RETURNING id`, f.storeAID, catID, fmt.Sprintf("cf-%d", i)).Scan(&id); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
	}
	vc := &products.ViewCounter{Redis: f.e.d.Redis, DB: f.e.d.DB, Log: f.e.d.Log}
	cs := buyers(t, f, 6, "flush")

	stop := make(chan struct{})
	var flushMu sync.Mutex
	var flushErr error
	setErr := func(err error) {
		flushMu.Lock()
		defer flushMu.Unlock()
		if err != nil && flushErr == nil {
			flushErr = err
		}
	}
	var hits int64
	var fwg sync.WaitGroup
	for w := 0; w < 3; w++ { // три инстанса API
		fwg.Add(1)
		go func() {
			defer fwg.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}
				for _, id := range ids {
					vc.Hit(ctx, id)
					atomic.AddInt64(&hits, 1)
				}
				setErr(products.RecordFittingAdds(ctx, f.e.d.DB, ids, onesFor(ids)))
				setErr(vc.Flush(ctx))
			}
		}()
	}
	var wg sync.WaitGroup
	var mu sync.Mutex
	var bad []string
	for i, c := range cs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for k := 0; k < 3; k++ {
				var items []map[string]any
				for _, id := range ids {
					items = append(items, item(id.String(), 1))
				}
				if r := c.doH("POST", "/v1/orders", checkoutBody(items...), idem(fmt.Sprintf("flush-key-%d-%d", i, k))); r.Status != 201 {
					mu.Lock()
					bad = append(bad, fmt.Sprintf("%d: %s", r.Status, r.Raw))
					mu.Unlock()
				}
			}
		}()
	}
	wg.Wait()
	close(stop)
	fwg.Wait()
	if err := vc.Flush(ctx); err != nil {
		t.Fatal(err)
	}
	if flushErr != nil {
		t.Fatalf("flush: %v", flushErr)
	}
	if len(bad) > 0 {
		t.Fatalf("checkout failures: %v", bad)
	}
	var views int64
	_ = f.e.d.DB.QueryRow(ctx, `SELECT COALESCE(sum(views_count), 0) FROM products WHERE id = ANY($1)`, ids).Scan(&views)
	if views != atomic.LoadInt64(&hits) {
		t.Fatalf("views lost: db %d, hits %d", views, hits)
	}
}

func onesFor(ids []uuid.UUID) []int64 {
	out := make([]int64, len(ids))
	for i := range out {
		out[i] = 1
	}
	return out
}
