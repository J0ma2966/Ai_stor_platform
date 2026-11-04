package products

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/xuri/excelize/v2"

	"github.com/boutique-market/backend/internal/audit"
	"github.com/boutique-market/backend/internal/auth"
	"github.com/boutique-market/backend/internal/httpx"
	"github.com/boutique-market/backend/internal/media"
	"github.com/boutique-market/backend/internal/platform/db"
)

// Импорт товаров из Excel (ТЗ 5.4). Шаблон: GET /v1/store/products/import/template.
//
// Столбцы (первая строка — заголовки, порядок любой):
//
//	sku, title, description, category, brand, price, in_stock
//
// category — slug категории (headwear, tops, bottoms, shoes) или её название на ru/uz;
// price — цена в сумах (целое или с дробной частью до тийина);
// in_stock — да/нет, ha/yo'q, yes/no, 1/0.
//
// Товар ищется по артикулу (sku): новый создаётся черновиком, существующий
// обновляется (кроме заблокированных модератором). Пустой sku — всегда новый
// товар с автоартикулом (NextSKU); выданные артикулы возвращаются в generated. Фотографии загружаются
// отдельно и сопоставляются по артикулу в имени файла (bulkImages).
const (
	MaxImportBytes = 5 << 20
	MaxImportRows  = 2000
)

var importColumns = []string{"sku", "title", "description", "category", "brand", "price", "in_stock"}

type ImportError struct {
	Row     int    `json:"row"` // номер строки в Excel (с 2)
	Field   string `json:"field,omitempty"`
	Code    string `json:"code"`
	Message string `json:"message,omitempty"`
}

type ImportResult struct {
	Total   int           `json:"total"`
	Created int           `json:"created"`
	Updated int           `json:"updated"`
	Failed  int           `json:"failed"`
	Errors  []ImportError `json:"errors"`
	// Generated — автоартикулы, выданные строкам с пустым sku (чтобы назвать по ним фото).
	Generated []GeneratedSKU `json:"generated,omitempty"`
}

type GeneratedSKU struct {
	Row int    `json:"row"`
	SKU string `json:"sku"`
}

type importRow struct {
	row         int
	sku         string
	title       string
	description string
	categoryID  uuid.UUID
	brand       string
	price       int64
	inStock     bool
}

func (h *Handlers) importTemplate(w http.ResponseWriter, r *http.Request) {
	f := excelize.NewFile()
	defer f.Close()
	sh := f.GetSheetName(0)
	for i, c := range importColumns {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		_ = f.SetCellValue(sh, cell, c)
	}
	example := []any{"A-100", "Пальто шерстяное", "Шерсть 80%, подкладка", "tops", "Acme", 1500000, "да"}
	for i, v := range example {
		cell, _ := excelize.CoordinatesToCellName(i+1, 2)
		_ = f.SetCellValue(sh, cell, v)
	}
	_ = f.SetColWidth(sh, "A", "G", 22)
	// справочник категорий на втором листе
	_, _ = f.NewSheet("categories")
	rows, err := h.DB.Query(r.Context(), `SELECT slug, name_ru, name_uz FROM categories WHERE is_active ORDER BY sort, slug`)
	if err == nil {
		i := 1
		_ = f.SetSheetRow("categories", "A1", &[]any{"category", "ru", "uz"})
		for rows.Next() {
			var slug, ru, uz string
			if rows.Scan(&slug, &ru, &uz) == nil {
				i++
				_ = f.SetSheetRow("categories", "A"+strconv.Itoa(i), &[]any{slug, ru, uz})
			}
		}
		rows.Close()
	}
	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	w.Header().Set("Content-Disposition", `attachment; filename="products-template.xlsx"`)
	_, _ = w.Write(buf.Bytes())
}

// importExcel: тело запроса — сам .xlsx файл.
func (h *Handlers) importExcel(w http.ResponseWriter, r *http.Request) {
	p := auth.MustFrom(r.Context())
	data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, MaxImportBytes))
	if err != nil {
		httpx.Fail(w, r, httpx.NewError(http.StatusRequestEntityTooLarge, "file_too_large", "file is larger than 5 MB"))
		return
	}
	// xlsx — это zip: проверяем сигнатуру и ограничиваем распаковку (защита от zip-бомбы)
	if !bytes.HasPrefix(data, []byte("PK\x03\x04")) {
		httpx.Fail(w, r, httpx.NewError(http.StatusUnsupportedMediaType, "unsupported_type", "expected .xlsx file"))
		return
	}
	f, err := excelize.OpenReader(bytes.NewReader(data), excelize.Options{UnzipSizeLimit: 64 << 20, UnzipXMLSizeLimit: 32 << 20})
	if err != nil {
		httpx.Fail(w, r, httpx.NewError(422, "bad_file", "cannot read .xlsx file"))
		return
	}
	defer f.Close()
	rows, err := f.GetRows(f.GetSheetName(0))
	if err != nil {
		httpx.Fail(w, r, httpx.NewError(422, "bad_file", "cannot read first sheet"))
		return
	}
	cats, err := h.categoryIndex(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	parsed, res, ferr := parseImport(rows, cats)
	if ferr != nil {
		httpx.Fail(w, r, ferr)
		return
	}
	if len(parsed) > 0 {
		err = db.Tx(r.Context(), h.DB, func(tx pgx.Tx) error {
			for _, it := range parsed {
				var created bool
				if it.sku == "" { // пустой артикул в строке — новый товар с автоартикулом
					sku, err := NextSKU(r.Context(), tx, p.StoreID)
					if err != nil {
						return err
					}
					it.sku = sku
					res.Generated = append(res.Generated, GeneratedSKU{Row: it.row, SKU: sku})
				}
				err := tx.QueryRow(r.Context(), `
					INSERT INTO products (store_id, category_id, sku, title, description, brand, price, in_stock)
					VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
					ON CONFLICT (store_id, sku) DO UPDATE SET
					  category_id = EXCLUDED.category_id, title = EXCLUDED.title, description = EXCLUDED.description,
					  brand = EXCLUDED.brand, price = EXCLUDED.price, in_stock = EXCLUDED.in_stock
					WHERE products.status <> 'blocked'
					RETURNING (xmax = 0)`,
					p.StoreID, it.categoryID, it.sku, it.title, it.description, it.brand, it.price, it.inStock).Scan(&created)
				switch {
				case errors.Is(err, pgx.ErrNoRows): // существующий заблокированный товар
					res.Errors = append(res.Errors, ImportError{Row: it.row, Field: "sku", Code: "product_blocked"})
					res.Failed++
				case err != nil:
					return err
				case created:
					res.Created++
				default:
					res.Updated++
				}
			}
			return audit.Log(r.Context(), tx, "product.import", "store", p.StoreID.String(),
				map[string]any{"created": res.Created, "updated": res.Updated, "failed": res.Failed})
		})
		if err != nil {
			httpx.Fail(w, r, err)
			return
		}
	}
	httpx.JSON(w, http.StatusOK, res)
}

func (h *Handlers) categoryIndex(r *http.Request) (map[string]uuid.UUID, error) {
	rows, err := h.DB.Query(r.Context(), `SELECT id, slug, name_ru, name_uz FROM categories WHERE is_active`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	m := map[string]uuid.UUID{}
	for rows.Next() {
		var id uuid.UUID
		var slug, ru, uz string
		if err := rows.Scan(&id, &slug, &ru, &uz); err != nil {
			return nil, err
		}
		for _, k := range []string{slug, ru, uz} {
			m[norm(k)] = id
		}
	}
	return m, rows.Err()
}

func norm(s string) string { return strings.ToLower(strings.TrimSpace(s)) }

// parseImport проверяет строки и собирает ошибки по каждой. Ошибка формата всего
// файла (нет нужных столбцов, слишком много строк) возвращается отдельно.
func parseImport(rows [][]string, cats map[string]uuid.UUID) ([]importRow, *ImportResult, error) {
	res := &ImportResult{Errors: []ImportError{}}
	if len(rows) == 0 {
		return nil, res, httpx.NewError(422, "bad_file", "file is empty")
	}
	col := map[string]int{}
	for i, hname := range rows[0] {
		col[norm(hname)] = i
	}
	var missing []string
	for _, c := range importColumns {
		if _, ok := col[c]; !ok {
			missing = append(missing, c)
		}
	}
	if len(missing) > 0 {
		return nil, res, &httpx.Error{Status: 422, Code: "missing_columns", Message: "missing columns: " + strings.Join(missing, ", "),
			Fields: map[string]string{"columns": strings.Join(missing, ",")}}
	}
	if len(rows)-1 > MaxImportRows {
		return nil, res, httpx.NewError(422, "too_many_rows", fmt.Sprintf("at most %d rows per file", MaxImportRows))
	}
	seen := map[string]int{}
	var out []importRow
	for i, raw := range rows[1:] {
		rowNum := i + 2
		get := func(c string) string {
			if j := col[c]; j < len(raw) {
				return strings.TrimSpace(raw[j])
			}
			return ""
		}
		empty := true
		for _, c := range importColumns {
			if get(c) != "" {
				empty = false
			}
		}
		if empty {
			continue
		}
		res.Total++
		it := importRow{row: rowNum, sku: get("sku"), title: get("title"), description: get("description"), brand: get("brand")}
		var errs []ImportError
		bad := func(field, code string) { errs = append(errs, ImportError{Row: rowNum, Field: field, Code: code}) }
		textField := func(field, v string, max int) {
			switch n := utf8.RuneCountInString(v); {
			case n == 0:
				bad(field, "required")
			case n > max:
				bad(field, "too_long")
			}
		}
		if utf8.RuneCountInString(it.sku) > 64 { // пустой артикул допустим: выдадим автоматически
			bad("sku", "too_long")
		}
		textField("title", it.title, 300)
		textField("description", it.description, 10000)
		textField("brand", it.brand, 120)
		if prev, dup := seen[norm(it.sku)]; dup && it.sku != "" {
			errs = append(errs, ImportError{Row: rowNum, Field: "sku", Code: "duplicate", Message: "same sku in row " + strconv.Itoa(prev)})
		} else {
			seen[norm(it.sku)] = rowNum
		}
		if id, ok := cats[norm(get("category"))]; ok {
			it.categoryID = id
		} else {
			bad("category", "unknown")
		}
		if v, ok := parsePrice(get("price")); ok {
			it.price = v
		} else {
			bad("price", "invalid")
		}
		if v, ok := parseBool(get("in_stock")); ok {
			it.inStock = v
		} else {
			bad("in_stock", "invalid")
		}
		if len(errs) > 0 {
			res.Errors = append(res.Errors, errs...)
			res.Failed++
			continue
		}
		out = append(out, it)
	}
	return out, res, nil
}

// parsePrice: сумы → тийины. Принимает "1500000", "1 500 000", "1500000.50", "1500000,5".
func parsePrice(s string) (int64, bool) {
	s = strings.NewReplacer(" ", "", " ", "", "сум", "", "so'm", "", "UZS", "").Replace(strings.TrimSpace(s))
	s = strings.ReplaceAll(s, ",", ".")
	if s == "" {
		return 0, false
	}
	whole, frac, _ := strings.Cut(s, ".")
	if len(frac) > 2 {
		return 0, false
	}
	for len(frac) < 2 {
		frac += "0"
	}
	w, err1 := strconv.ParseInt(whole, 10, 64)
	f, err2 := strconv.ParseInt(frac, 10, 64)
	if err1 != nil || err2 != nil || w < 0 || w > 1_000_000_000_000 {
		return 0, false
	}
	v := w*100 + f
	return v, v > 0
}

func parseBool(s string) (bool, bool) {
	switch norm(s) {
	case "да", "yes", "1", "true", "ha", "bor", "в наличии", "+":
		return true, true
	case "нет", "no", "0", "false", "yo'q", "yoq", "yo‘q", "нет в наличии", "-":
		return false, true
	}
	return false, false
}

// ---------- массовая загрузка фото ----------

type bulkItem struct {
	Filename string `json:"filename" validate:"required,max=255"`
	Key      string `json:"key" validate:"required,max=200"`
}

type bulkReq struct {
	Items []bulkItem `json:"items" validate:"required,min=1,max=300,dive"`
}

type BulkResult struct {
	Filename  string     `json:"filename"`
	ProductID *uuid.UUID `json:"product_id,omitempty"`
	SKU       string     `json:"sku,omitempty"`
	Angle     string     `json:"angle,omitempty"`
	Error     string     `json:"error,omitempty"` // bad_name | unknown_sku | not_owned | product_blocked
}

var angleAliases = map[string]string{
	"front": "front", "f": "front", "1": "front", "спереди": "front", "old": "front",
	"side": "side", "s": "side", "2": "side", "сбоку": "side", "yon": "side", "yon_tomon": "side",
	"back": "back", "b": "back", "3": "back", "сзади": "back", "orqa": "back",
}

// ParseImageName: "A-100_front.jpg" → ("A-100", "front"). Разделитель — последний "_".
func ParseImageName(name string) (sku, angle string, ok bool) {
	base := path.Base(strings.ReplaceAll(name, `\`, "/"))
	base = strings.TrimSuffix(base, path.Ext(base))
	i := strings.LastIndexByte(base, '_')
	if i <= 0 || i == len(base)-1 {
		return "", "", false
	}
	a, ok := angleAliases[norm(base[i+1:])]
	if !ok {
		return "", "", false
	}
	return strings.TrimSpace(base[:i]), a, true
}

// bulkImages: файлы загружены через POST /v1/uploads?purpose=product, здесь их
// ключи привязываются к товарам по артикулу и ракурсу в имени файла.
func (h *Handlers) bulkImages(w http.ResponseWriter, r *http.Request) {
	p := auth.MustFrom(r.Context())
	var req bulkReq
	if err := httpx.Decode(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	results := make([]BulkResult, len(req.Items))
	var replaced []string
	err := db.Tx(r.Context(), h.DB, func(tx pgx.Tx) error {
		for i, it := range req.Items {
			res := BulkResult{Filename: it.Filename}
			sku, angle, ok := ParseImageName(it.Filename)
			switch {
			case !media.OwnsKey(p.UserID, media.PurposeProductImage, it.Key):
				res.Error = "not_owned"
			case !ok:
				res.Error = "bad_name"
			default:
				res.SKU, res.Angle = sku, angle
				var id uuid.UUID
				var status string
				err := tx.QueryRow(r.Context(), `SELECT id, status::text FROM products WHERE store_id = $1 AND lower(sku) = lower($2)`,
					p.StoreID, sku).Scan(&id, &status)
				switch {
				case errors.Is(err, pgx.ErrNoRows):
					res.Error = "unknown_sku"
				case err != nil:
					return err
				case status == "blocked":
					res.Error = "product_blocked"
				default:
					var prev *string
					if err := tx.QueryRow(r.Context(), `SELECT key FROM product_images WHERE product_id = $1 AND angle = $2`, id, angle).Scan(&prev); err != nil && !errors.Is(err, pgx.ErrNoRows) {
						return err
					}
					if prev != nil && *prev != it.Key {
						replaced = append(replaced, *prev)
					}
					if _, err := tx.Exec(r.Context(), `INSERT INTO product_images (product_id, angle, key) VALUES ($1, $2, $3)
						ON CONFLICT (product_id, angle) DO UPDATE SET key = EXCLUDED.key`, id, angle, it.Key); err != nil {
						return err
					}
					res.ProductID = &id
				}
			}
			results[i] = res
		}
		return nil
	})
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	h.deleteUnreferenced(r, replaced)
	httpx.JSON(w, http.StatusOK, results)
}
