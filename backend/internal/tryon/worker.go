package tryon

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"

	"github.com/boutique-market/backend/internal/media"
	"github.com/boutique-market/backend/internal/platform/db"
	"github.com/boutique-market/backend/internal/tokens"
)

var (
	mJobs = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "tryon_generations_total", Help: "AI try-on jobs by outcome (succeeded, failed, retry)",
	}, []string{"outcome", "provider"})
	mDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name: "tryon_generation_seconds", Help: "AI try-on job processing time",
		Buckets: []float64{2, 5, 10, 20, 30, 45, 60, 90, 120, 180, 300},
	}, []string{"provider"})
	mQueueWait = promauto.NewHistogram(prometheus.HistogramOpts{
		Name: "tryon_queue_wait_seconds", Help: "Time from enqueue to first pickup",
		Buckets: []float64{0.5, 1, 2, 5, 10, 30, 60, 120, 300, 600},
	})
)

// NotifyFunc — уведомление клиента о завершении генерации (kind: generation_succeeded,
// generation_failed). Вызывается в транзакции смены статуса.
// main.go подставляет notify.Send.
type NotifyFunc func(ctx context.Context, tx pgx.Tx, userID uuid.UUID, kind string, data map[string]any) error

// Worker обрабатывает очередь генераций. Безопасно запускать в нескольких процессах:
// задание берётся через FOR UPDATE SKIP LOCKED и «арендуется» до lease_until;
// если воркер упал, после истечения аренды задание подхватит другой.
type Worker struct {
	DB          *pgxpool.Pool
	Store       media.Storage
	Provider    Provider
	Log         *slog.Logger
	Notify      NotifyFunc
	Concurrency int
	Timeout     time.Duration // на одно задание (все ракурсы)
	MaxAttempts int
	Poll        time.Duration // пауза, когда очередь пуста
}

type job struct {
	id, userID uuid.UUID
	productIDs []uuid.UUID
	photoKeys  []string
	body       Body
	gender     *string
	cost       int64
	attempts   int
	createdAt  time.Time
	startedAt  time.Time
}

func (w *Worker) lease() time.Duration { return w.Timeout + time.Minute }

// Run запускает Concurrency циклов обработки до отмены ctx.
func (w *Worker) Run(ctx context.Context) {
	n := max(w.Concurrency, 1)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			w.loop(ctx)
		}()
	}
	w.Log.Info("tryon worker started", slog.Int("concurrency", n), slog.String("provider", w.Provider.Name()))
	wg.Wait()
}

func (w *Worker) loop(ctx context.Context) {
	poll := w.Poll
	if poll == 0 {
		poll = time.Second
	}
	for ctx.Err() == nil {
		did, err := w.RunOnce(ctx)
		if err != nil && ctx.Err() == nil {
			w.Log.Error("tryon worker", slog.String("err", err.Error()))
		}
		if did {
			continue
		}
		// джиттер, чтобы воркеры разных процессов не опрашивали БД синхронно
		d := poll/2 + jitter(poll)
		select {
		case <-ctx.Done():
		case <-time.After(d):
		}
	}
}

// RunOnce берёт одно задание и обрабатывает его. false — очередь пуста.
func (w *Worker) RunOnce(ctx context.Context) (bool, error) {
	j, err := w.claim(ctx)
	if err != nil || j == nil {
		return false, err
	}
	if j.attempts == 1 {
		mQueueWait.Observe(j.startedAt.Sub(j.createdAt).Seconds())
	}
	if j.attempts > max(w.MaxAttempts, 1) {
		// задание «зависло» (воркер умирал на нём) больше допустимого числа раз
		return true, w.fail(ctx, j, permanent(CodeProviderFailure, "attempts exhausted"))
	}
	start := time.Now()
	jctx, cancel := context.WithTimeout(ctx, w.Timeout)
	result, err := w.process(jctx, j)
	cancel()
	mDuration.WithLabelValues(w.Provider.Name()).Observe(time.Since(start).Seconds())
	if err != nil {
		if ctx.Err() != nil {
			// остановка процесса: аренда истечёт, задание подхватят повторно
			return true, nil
		}
		if IsPermanent(err) || j.attempts >= max(w.MaxAttempts, 1) {
			return true, w.fail(ctx, j, err)
		}
		return true, w.retry(ctx, j, err)
	}
	return true, w.succeed(ctx, j, result)
}

func (w *Worker) claim(ctx context.Context) (*job, error) {
	var j job
	var body []byte
	err := w.DB.QueryRow(ctx, `
		UPDATE generations g SET status = 'running', attempts = g.attempts + 1,
		       started_at = COALESCE(g.started_at, now()), lease_until = now() + make_interval(secs => $1),
		       provider = $2
		FROM (SELECT id FROM generations
		      WHERE (status = 'queued' AND next_run_at <= now()) OR (status = 'running' AND lease_until < now())
		      ORDER BY next_run_at LIMIT 1 FOR UPDATE SKIP LOCKED) c
		WHERE g.id = c.id
		RETURNING g.id, g.user_id, g.product_ids, g.photo_keys, g.body, g.gender::text, g.cost, g.attempts, g.created_at, g.started_at`,
		w.lease().Seconds(), w.Provider.Name()).
		Scan(&j.id, &j.userID, &j.productIDs, &j.photoKeys, &body, &j.gender, &j.cost, &j.attempts, &j.createdAt, &j.startedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	_ = json.Unmarshal(body, &j.body)
	return &j, nil
}

type productInput struct {
	zone, title, brand string
	set                bool
	keys               map[string]string // angle → key
}

func (w *Worker) loadProducts(ctx context.Context, ids []uuid.UUID) ([]productInput, error) {
	rows, err := w.DB.Query(ctx, `
		SELECT c.zone::text, p.title, p.brand, p.is_set,
		       COALESCE((SELECT jsonb_object_agg(pi.angle, pi.key) FROM product_images pi WHERE pi.product_id = p.id), '{}')
		FROM products p JOIN categories c ON c.id = p.category_id
		WHERE p.id = ANY($1)`, ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []productInput
	for rows.Next() {
		var p productInput
		var keys []byte
		if err := rows.Scan(&p.zone, &p.title, &p.brand, &p.set, &keys); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(keys, &p.keys)
		if p.keys["front"] == "" {
			continue // без фото товар нечего надевать
		}
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(out) == 0 {
		return nil, permanent(CodeProductMissing, "no products with photos")
	}
	return out, nil
}

func (w *Worker) get(ctx context.Context, key, code string) ([]byte, error) {
	b, err := w.Store.Get(ctx, key)
	if errors.Is(err, media.ErrObjectNotFound) {
		return nil, permanent(code, "object missing")
	}
	return b, err
}

// process: загрузить входные данные; для CompositeProvider — один вызов без склейки,
// для остальных — вызов провайдера на каждый ракурс параллельно и склейка Compose.
func (w *Worker) process(ctx context.Context, j *job) ([]byte, error) {
	if len(j.photoKeys) != len(Angles) {
		return nil, permanent(CodePhotoMissing, "expected %d photos", len(Angles))
	}
	prods, err := w.loadProducts(ctx, j.productIDs)
	if err != nil {
		return nil, err
	}
	imgCache := map[string][]byte{}
	garmentsFor := func(angle string) ([]Garment, error) {
		var gs []Garment
		for _, p := range prods {
			key := p.keys[angle]
			if key == "" {
				key = p.keys["front"]
			}
			img, ok := imgCache[key]
			if !ok {
				if img, err = w.get(ctx, key, CodeProductMissing); err != nil {
					return nil, err
				}
				imgCache[key] = img
			}
			gs = append(gs, Garment{Zone: p.zone, Title: p.title, Brand: p.brand, Image: img, Set: p.set})
		}
		return gs, nil
	}
	if err := w.checkTransferConsent(ctx, j); err != nil {
		return nil, err
	}
	if cp, ok := w.Provider.(CompositeProvider); ok {
		return w.processComposite(ctx, j, cp, prods)
	}
	reqs := make([]Request, len(Angles))
	for i, a := range Angles {
		person, err := w.get(ctx, j.photoKeys[i], CodePhotoMissing)
		if err != nil {
			return nil, err
		}
		gs, err := garmentsFor(a)
		if err != nil {
			return nil, err
		}
		reqs[i] = Request{Angle: a, Person: person, Garments: gs, Body: j.body}
		if j.gender != nil {
			reqs[i].Gender = *j.gender
		}
	}
	views := make([][]byte, len(reqs))
	errs := make([]error, len(reqs))
	var wg sync.WaitGroup
	for i := range reqs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			views[i], errs[i] = w.Provider.Generate(ctx, reqs[i])
		}()
	}
	wg.Wait()
	// постоянная ошибка важнее временной: повтор её не исправит
	for _, e := range errs {
		if IsPermanent(e) {
			return nil, e
		}
	}
	if err := errors.Join(errs...); err != nil {
		return nil, err
	}
	out, err := Compose(views)
	if err != nil {
		return nil, permanent(CodeProviderFailure, "compose: %v", err)
	}
	return out, nil
}

// checkTransferConsent — последняя проверка перед отправкой фото внешнему провайдеру:
// согласие на передачу должно быть дано именно ему (на случай смены TRYON_PROVIDER,
// пока задание стояло в очереди). Без согласия фото не отправляются, токены возвращаются.
func (w *Worker) checkTransferConsent(ctx context.Context, j *job) error {
	name := w.Provider.Name()
	if TransferRecipients[name] == "" {
		return nil
	}
	var ok bool
	if err := w.DB.QueryRow(ctx, `SELECT COALESCE(tryon_transfer_provider = $2, false) FROM users WHERE id = $1`,
		j.userID, name).Scan(&ok); err != nil {
		return err
	}
	if !ok {
		return permanent(CodeTransferConsent, "no consent to send photos to %s", name)
	}
	return nil
}

// processComposite — сценарий «один запрос — одно изображение» (OpenAI): три фото
// клиента и фронтальные фото всех выбранных вещей уходят в ОДИН вызов провайдера,
// результат сохраняется как есть, без Compose.
func (w *Worker) processComposite(ctx context.Context, j *job, cp CompositeProvider, prods []productInput) ([]byte, error) {
	photos := make([][]byte, len(Angles))
	for i := range Angles {
		b, err := w.get(ctx, j.photoKeys[i], CodePhotoMissing)
		if err != nil {
			return nil, err
		}
		photos[i] = b
	}
	req := CompositeRequest{FrontPhoto: photos[0], SidePhoto: photos[1], BackPhoto: photos[2], Body: j.body}
	if j.gender != nil {
		req.Gender = *j.gender
	}
	for _, p := range prods {
		img, err := w.get(ctx, p.keys["front"], CodeProductMissing)
		if err != nil {
			return nil, err
		}
		req.Garments = append(req.Garments, Garment{Zone: p.zone, Title: p.title, Brand: p.brand, Image: img, Set: p.set})
	}
	return cp.GenerateComposite(ctx, req)
}

// owned — UPDATE применяется, только если задание всё ещё за этой попыткой
// (иначе аренда истекла и задание взял другой воркер).
const owned = `id = $1 AND status = 'running' AND attempts = $2`

const (
	sqlSucceed = `UPDATE generations SET status = 'succeeded', result_key = $3, finished_at = now(),
			lease_until = NULL, error = NULL, error_code = NULL WHERE ` + owned
	sqlRetry = `UPDATE generations SET status = 'queued', lease_until = NULL,
		next_run_at = now() + make_interval(secs => $3), error = left($4, 1000) WHERE ` + owned
	sqlFail = `UPDATE generations SET status = 'failed', finished_at = now(), lease_until = NULL,
			error = left($3, 1000), error_code = $4 WHERE ` + owned
)

func (w *Worker) succeed(ctx context.Context, j *job, img []byte) error {
	key := media.NewKey(j.userID, media.PurposeTryOnResult)
	if err := w.Store.Put(ctx, key, img, "image/jpeg"); err != nil {
		return w.retry(ctx, j, fmt.Errorf("store result: %w", err))
	}
	var applied bool
	err := db.Tx(ctx, w.DB, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, sqlSucceed, j.id, j.attempts, key)
		if err != nil || tag.RowsAffected() == 0 {
			return err
		}
		applied = true
		return w.notify(ctx, tx, j, "generation_succeeded")
	})
	if err != nil || !applied {
		_ = w.Store.Delete(context.WithoutCancel(ctx), key)
		return err
	}
	mJobs.WithLabelValues("succeeded", w.Provider.Name()).Inc()
	w.Log.Info("tryon succeeded", slog.String("generation_id", j.id.String()), slog.Int("attempt", j.attempts))
	return nil
}

func backoff(attempt int) time.Duration {
	d := time.Duration(1<<min(attempt, 6)) * 5 * time.Second // 10s, 20s, 40s ...
	return d + jitter(d/2)
}

func (w *Worker) retry(ctx context.Context, j *job, cause error) error {
	_, err := w.DB.Exec(ctx, sqlRetry,
		j.id, j.attempts, backoff(j.attempts).Seconds(), cause.Error())
	mJobs.WithLabelValues("retry", w.Provider.Name()).Inc()
	w.Log.Warn("tryon retry", slog.String("generation_id", j.id.String()), slog.Int("attempt", j.attempts), slog.String("err", cause.Error()))
	return err
}

// fail помечает генерацию ошибочной и возвращает токены (ТЗ 6.5, 8.1). Повторный
// возврат невозможен: уникальный индекс token_ledger_ref_once по (op, generation).
func (w *Worker) fail(ctx context.Context, j *job, cause error) error {
	err := db.Tx(ctx, w.DB, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, sqlFail, j.id, j.attempts, cause.Error(), ErrorCode(cause))
		if err != nil || tag.RowsAffected() == 0 {
			return err
		}
		if err := refund(ctx, tx, j.id, j.userID, j.cost); err != nil {
			return err
		}
		return w.notify(ctx, tx, j, "generation_failed")
	})
	if err != nil {
		return err
	}
	mJobs.WithLabelValues("failed", w.Provider.Name()).Inc()
	w.Log.Warn("tryon failed", slog.String("generation_id", j.id.String()), slog.String("code", ErrorCode(cause)), slog.String("err", cause.Error()))
	return nil
}

func refund(ctx context.Context, tx pgx.Tx, genID, userID uuid.UUID, cost int64) error {
	if cost <= 0 {
		return nil
	}
	_, err := tokens.Apply(ctx, tx, tokens.Entry{UserID: userID, Op: tokens.OpGenerationRefund, Amount: cost,
		ReferenceType: "generation", ReferenceID: &genID, Reason: "generation failed"})
	if errors.Is(err, tokens.ErrDuplicate) {
		return nil
	}
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `UPDATE generations SET refunded = true WHERE id = $1`, genID)
	return err
}

func (w *Worker) notify(ctx context.Context, tx pgx.Tx, j *job, kind string) error {
	if w.Notify == nil {
		return nil
	}
	return w.Notify(ctx, tx, j.userID, kind, map[string]any{"generation_id": j.id})
}

// jitter — случайная пауза [0, max).
func jitter(max time.Duration) time.Duration {
	if max <= 0 {
		return 0
	}
	var b [8]byte
	_, _ = rand.Read(b[:])
	return time.Duration(binary.LittleEndian.Uint64(b[:]) % uint64(max)) // #nosec G115 -- max > 0
}
