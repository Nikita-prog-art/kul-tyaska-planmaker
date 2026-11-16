package web_test

import (
	"encoding/json"
	"io"
	"math"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"kultyaska/internal/config"
	"kultyaska/internal/plan"
	"kultyaska/internal/store"
	"kultyaska/internal/web"
)

type client struct {
	t    *testing.T
	srv  *httptest.Server
	http *http.Client
}

func newClient(t *testing.T) *client {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "db.json"))
	if err != nil {
		t.Fatalf("хранилище: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	cfg := config.Config{Addr: "127.0.0.1:0", DBPath: ":memory:", SessionTTL: time.Hour, Admins: []string{"archi"}}
	srv, err := web.New(cfg, st)
	if err != nil {
		t.Fatalf("сервер: %v", err)
	}
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	jar, _ := cookiejar.New(nil)
	return &client{t: t, srv: ts, http: &http.Client{Jar: jar, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}
}

func (c *client) do(method, path string, form url.Values) *http.Response {
	c.t.Helper()
	var body io.Reader
	if form != nil {
		body = strings.NewReader(form.Encode())
	}
	req, err := http.NewRequest(method, c.srv.URL+path, body)
	if err != nil {
		c.t.Fatal(err)
	}
	if form != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		c.t.Fatal(err)
	}
	return resp
}

func (c *client) getBody(path string) (int, string) {
	c.t.Helper()
	resp := c.do("GET", path, nil)
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(raw)
}

func (c *client) post(path string, form url.Values) *http.Response {
	c.t.Helper()
	resp := c.do("POST", path, form)
	_, _ = io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	return resp
}

// postJSON — сохранение из конструктора: тот же form-urlencoded, но с
// заголовком X-KT-Save, по которому сервер отвечает JSON-ом вместо редиректа.
func (c *client) postJSON(path string, form url.Values) (*http.Response, string) {
	c.t.Helper()
	body := ""
	if form != nil {
		body = form.Encode()
	}
	req, err := http.NewRequest("POST", c.srv.URL+path, strings.NewReader(body))
	if err != nil {
		c.t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("X-KT-Save", "fetch")
	resp, err := c.http.Do(req)
	if err != nil {
		c.t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	return resp, string(raw)
}

func TestPagesRequireAuth(t *testing.T) {
	c := newClient(t)
	resp := c.post("/plans", nil)
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("без сессии /plans должен редиректить, получил %d", resp.StatusCode)
	}
	if loc := resp.Header.Get("Location"); !strings.HasPrefix(loc, "/login") {
		t.Errorf("редирект не на логин: %s", loc)
	}
}

func TestFullFlow(t *testing.T) {
	c := newClient(t)

	// Регистрация: первый пользователь становится админом.
	resp := c.post("/register", url.Values{
		"nick": {"archi"}, "discord": {"@archi"}, "password": {"verysecret1"}, "password2": {"verysecret1"},
	})
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("регистрация: код %d", resp.StatusCode)
	}
	code, body := c.getBody("/plans")
	if code != 200 {
		t.Fatalf("/plans: код %d", code)
	}
	if !strings.Contains(body, "Мои планы") || !strings.Contains(body, "08-zaskoruzlye-piramidy") {
		t.Fatal("на странице планов нет списка каркасов")
	}

	// Создание плана из каркаса.
	resp = c.post("/plans", url.Values{"title": {"Тест"}, "emoji": {"🩸"}, "preset": {"08-zaskoruzlye-piramidy"}})
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("создание плана: код %d", resp.StatusCode)
	}
	planURL := resp.Header.Get("Location")
	if !strings.HasPrefix(planURL, "/plans/") {
		t.Fatalf("неожиданный редирект: %s", planURL)
	}

	// Конструктор отдаёт JSON плана и секции.
	code, body = c.getBody(planURL)
	if code != 200 {
		t.Fatalf("конструктор: код %d", code)
	}
	for _, want := range []string{`id="kt-plan"`, `id="kt-sections"`, "builder.js", "engine.js", "пинг систем"} {
		if !strings.Contains(body, want) {
			t.Errorf("в конструкторе нет %q", want)
		}
	}

	// Сохранение состояния и проверка экспорта. Осознанная правка магазина
	// разрешена, только если черновик собран на текущей ревизии плана.
	state := map[string]any{
		"base_updated_at": planFromPage(t, body)["updated_at"],
		"title":           "Праведность без контроля пола", "emoji": "🩸", "balance": 1605.24148,
		"decimals": true, "skeleton": "тест",
		"shop": []map[string]any{
			{"key": "lesson:a", "kind": "lesson", "section": "lessons", "title": "отмена детской смертности", "fixed": true, "cost": 1},
			{"key": "lesson:b", "kind": "lesson", "section": "lessons", "title": "демонические культиваторы — зло", "min_lesson": 1, "cost": 0},
		},
		"picks":     []map[string]any{{"key": "lesson:a"}, {"key": "lesson:b"}},
		"deferred":  []map[string]any{},
		"not_taken": []string{"lesson:b"},
		"ladders":   []map[string]any{{"key": "talent.die", "section": "talent", "title": "кость таланта", "from": 1, "to": 5, "step": 1, "value": 2, "cost": map[string]any{"type": "to_value", "rate": 1}}},
	}
	raw, _ := json.Marshal(state)
	resp = c.post(planURL, url.Values{"state": {string(raw)}})
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("сохранение: код %d", resp.StatusCode)
	}

	code, body = c.getBody(planURL + "/export")
	if code != 200 {
		t.Fatalf("экспорт: код %d", code)
	}
	if !strings.Contains(body, "Остаток:") || !strings.Contains(body, "отмена детской смертности") {
		t.Error("на странице экспорта нет текста плана")
	}

	resp = c.do("GET", planURL+"/export?format=txt", nil)
	txt, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/plain") {
		t.Errorf("ожидал text/plain, получил %s", ct)
	}
	if !strings.Contains(string(txt), "Невыученные: —") {
		t.Errorf("в .txt нет блока невыученных:\n%s", txt)
	}
	// Урок с фиксированной ценой не занимает номер.
	if !strings.Contains(string(txt), "1. демонические культиваторы — зло: 5") {
		t.Errorf("нумерация уроков считается неверно:\n%s", txt)
	}
}

func TestEscapingAndAdmin(t *testing.T) {
	c := newClient(t)
	c.post("/register", url.Values{
		"nick": {"archi"}, "discord": {"@archi#1"}, "password": {"verysecret1"}, "password2": {"verysecret1"},
	})

	resp := c.post("/plans", url.Values{"title": {`<script>alert(1)</script>`}})
	planURL := resp.Header.Get("Location")
	_, body := c.getBody(planURL)
	if strings.Contains(body, "<script>alert(1)</script>") {
		t.Error("название плана не экранируется — XSS")
	}

	code, body := c.getBody("/admin")
	if code != 200 {
		t.Fatalf("админка: код %d", code)
	}
	if !strings.Contains(body, "@archi#1") {
		t.Error("в админке нет дискорд-хендла")
	}

	// Пароль сбрасываем второму игроку: себе сброс закрывает свою же сессию.
	if resp := c.post("/logout", nil); resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("выход: код %d", resp.StatusCode)
	}
	if resp := c.post("/register", url.Values{
		"nick": {"guest"}, "discord": {"@guest#2"}, "password": {"verysecret1"}, "password2": {"verysecret1"},
	}); resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("регистрация второго игрока: код %d", resp.StatusCode)
	}
	if resp := c.post("/logout", nil); resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("выход гостя: код %d", resp.StatusCode)
	}
	if resp := c.post("/login", url.Values{"nick": {"archi"}, "password": {"verysecret1"}}); resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("повторный вход админом: код %d", resp.StatusCode)
	}
	if resp := c.post("/admin/reset", url.Values{"user_id": {userIDByNick(t, c, "@guest#2")}}); resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("сброс пароля: код %d", resp.StatusCode)
	}
	_, body = c.getBody("/admin")
	if !strings.Contains(body, "временный пароль") {
		t.Error("временный пароль не показан в админке")
	}
}

// userIDByNick выцепляет id пользователя по его дискорд-хендлу на странице админки.
func userIDByNick(t *testing.T, c *client, discord string) string {
	t.Helper()
	_, body := c.getBody("/admin")
	idx := strings.Index(body, discord)
	if idx < 0 {
		t.Fatalf("в админке нет %s", discord)
	}
	// Формы сброса идут в той же строке таблицы, сразу после дискорда.
	tail := body[idx:]
	const marker = `name="user_id" value="`
	i := strings.Index(tail, marker)
	if i < 0 {
		t.Fatalf("не нашёл id пользователя %s", discord)
	}
	rest := tail[i+len(marker):]
	return rest[:strings.Index(rest, `"`)]
}

func TestPresetServed(t *testing.T) {
	c := newClient(t)
	resp := c.do("GET", "/presets/08-zaskoruzlye-piramidy", nil)
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		t.Fatalf("каркас: код %d", resp.StatusCode)
	}
	sk, err := plan.ParseSkeleton(raw)
	if err != nil {
		t.Fatalf("каркас не разбирается: %v", err)
	}
	if len(sk.Shop) < 30 {
		t.Errorf("в каркасе мало позиций: %d", len(sk.Shop))
	}
}

// TestLife9Preset — каркас жизни 9 на континенте Исконного баланса: в нём
// только новое и ещё не купленное, накопители в том состоянии, которое
// пережило жизнь 8, и выгрузка собирается из него без проверок.
func TestLife9Preset(t *testing.T) {
	c := newClient(t)
	resp := c.do("GET", "/presets/09-kontinent-iskonnogo-balansa", nil)
	raw, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("каркас: код %d", resp.StatusCode)
	}
	sk, err := plan.ParseSkeleton(raw)
	if err != nil {
		t.Fatalf("каркас не разбирается: %v", err)
	}
	if sk.Name != "Жизнь 9 · континент Исконного баланса" {
		t.Errorf("название каркаса: %q", sk.Name)
	}
	if math.Abs(sk.Balance-458.19948) > 1e-9 {
		t.Errorf("баланс: %v", sk.Balance)
	}
	if sk.DeferredSlots != 2 {
		t.Errorf("слотов невыученных: %d, ждал 2", sk.DeferredSlots)
	}
	byKey := map[string]*plan.ShopItem{}
	for i := range sk.Shop {
		byKey[sk.Shop[i].Key] = &sk.Shop[i]
	}
	for key, cost := range map[string]float64{
		"lesson:skidka_na_skidku":        0,
		"lesson:plody_otravlennoy_palmy": 1,
		"lesson:vmeste_veselee_cao":      5,
		"item:diletantka_kastetov":       0,
		"item:luk_lish_raz_v_zhizni":     5,
		"revenge:monivong_dzhivan":       60,
		"folio:vykupit_silu_yunosti":     20,
	} {
		it := byKey[key]
		if it == nil {
			t.Errorf("в каркасе нет позиции %q", key)
			continue
		}
		if it.Cost != cost {
			t.Errorf("%s: цена %v, ждал %v", key, it.Cost, cost)
		}
	}
	for _, key := range []string{
		"item:pererozhdenie_dzhona", "item:ravnovesie_zla_i_dobra", "item:hardkor",
		"patch:otkluchenie_trenirovok", "folio:vykupit_miazmy", "folio:vzyat_posle_okraski",
		"lesson:ping_sistem", "lesson:poceluy_melisandry",
	} {
		if byKey[key] != nil {
			t.Errorf("в каркасе осталось купленное %q", key)
		}
	}
	if it := byKey["revenge:monivong_dzhivan"]; it == nil || it.Section != plan.SecRevenge {
		t.Errorf("месть не в своём разделе: %+v", it)
	}
	if it := byKey["item:skidka_na_tele"]; it == nil || len(it.Discounts) != 1 ||
		it.Discounts[0].Key != "lesson:skidka_na_skidku" || it.Discounts[0].Amount != 100 {
		t.Errorf("у постоянной скидки нет скидки за урок «скидка на скидку»: %+v", it)
	}
	// «Освоено» — пометка метода, а не фиксированная цена: перенесённый
	// невыученными уроками метод остаётся обычным нумеруемым уроком.
	if it := byKey["lesson:metod_neproglyadnogo_tumana"]; it == nil {
		t.Error("в каркасе нет метода Непроглядного Тумана")
	} else if it.Fixed {
		t.Errorf("метод Непроглядного Тумана не должен быть фиксированным: %+v", it)
	} else if !it.NoDefer {
		t.Error("метод перенесён невыученным — повторно отложить его нельзя")
	}
	if it := byKey["lesson:rodoslovnaya_raduzhnoy_foreli"]; it == nil {
		t.Error("в каркасе нет родословной Радужной Форели")
	} else if !it.NoDefer {
		t.Error("родословная перенесена невыученной — повторно отложить её нельзя")
	}
	// Младшая тренировка удешевляет старшую на свою цену.
	for key, want := range map[string]plan.PriceCut{
		"item:uchenica_schita":            {Key: "item:novichok_schita", Amount: 3},
		"item:novichok_cepa":              {Key: "item:diletantka_cepa", Amount: 2},
		"item:novichok_bulavy":            {Key: "item:diletantka_bulavy", Amount: 2},
		"item:novichok_bezoruzhnogo_boya": {Key: "item:diletantka_bezoruzhnogo_boya", Amount: 2},
	} {
		it := byKey[key]
		if it == nil || len(it.Discounts) != 1 || it.Discounts[0] != want {
			t.Errorf("%s: скидка за младшую тренировку %+v, а не %+v", key, it, want)
		}
	}
	if it := byKey["revenge:sosredotochennaya_mest"]; it == nil || it.MinLessons != 10 {
		t.Errorf("сосредоточенная месть не требует десяти уроков: %+v", it)
	}
	accs := map[string]*plan.Accumulator{}
	for i := range sk.Accumulators {
		accs[sk.Accumulators[i].Key] = &sk.Accumulators[i]
	}
	for key, want := range map[string][3]float64{
		"acc:gorozhane":      {1.05, 2, 25},
		"acc:okrestnosti_mu": {0.04, 1, 1},
		"acc:velikiy_uspeh":  {1, 2, 0},
		"acc:dzon_snou":      {1, 1, 0},
		"acc:ravnovesie":     {1, 1, 0},
		"acc:hardkor":        {-1, 1, 0},
	} {
		a := accs[key]
		if a == nil {
			t.Errorf("нет накопителя %q", key)
			continue
		}
		if a.Charges != want[0] || a.Max != want[1] || a.Rate != want[2] {
			t.Errorf("%s: %v/%v, +%v%%, ждал %v/%v, +%v%%", key, a.Charges, a.Max, a.Rate, want[0], want[1], want[2])
		}
	}
	ladders := map[string]*plan.LadderCfg{}
	for i := range sk.Ladders {
		ladders[sk.Ladders[i].Key] = &sk.Ladders[i]
	}
	for key, from := range map[string]float64{
		"talent.die": 25, "talent.dice_count": 5, "talent.flat": 6,
		"roots.min": 50, "chance.billion": 2, "chance.million_home": 1,
	} {
		l := ladders[key]
		if l == nil {
			t.Errorf("нет шкалы %q", key)
			continue
		}
		if l.From != from {
			t.Errorf("%s: уже есть %v, ждал %v", key, l.From, from)
		}
	}

	// План из каркаса: пять уроков, две фиксированные покупки, тренировка,
	// месть и фолиант. Проверки не должны сработать, а строки — быть короткими.
	c.post("/register", url.Values{
		"nick": {"life9"}, "discord": {"@life9"}, "password": {"verysecret1"}, "password2": {"verysecret1"},
	})
	resp = c.post("/plans", url.Values{"title": {"Жизнь 9"}, "emoji": {"🌊"}, "preset": {"09-kontinent-iskonnogo-balansa"}})
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("создание плана: код %d", resp.StatusCode)
	}
	planURL := resp.Header.Get("Location")
	_, body := c.getBody(planURL)
	st := planFromPage(t, body)
	st["decimals"] = true
	st["picks"] = []map[string]any{
		{"key": "lesson:skidka_na_skidku"},
		{"key": "lesson:pogibel_prizrakov"},
		{"key": "lesson:gluboko_nevyuchennye_uroki"},
		{"key": "lesson:rodoslovnaya_raduzhnoy_foreli"},
		{"key": "lesson:rodoslovnaya_pylayuschego_varana"},
		{"key": "lesson:plody_otravlennoy_palmy"},
		{"key": "lesson:vmeste_veselee_cao"},
		{"key": "item:diletantka_kastetov"},
		{"key": "revenge:monivong_dzhivan"},
		{"key": "folio:vzyat_1_foliant"},
		{"key": "folio:vykupit_tuman"},
	}
	for _, raw := range st["accumulators"].([]any) {
		a := raw.(map[string]any)
		if a["key"] == "acc:gorozhane" || a["key"] == "acc:dzon_snou" {
			a["activate"] = true
		}
	}
	for _, raw := range st["folios"].([]any) {
		f := raw.(map[string]any)
		if f["key"] == "folio:metod_neproglyadnogo_tumana" {
			f["take"] = true
		}
	}
	state, _ := json.Marshal(st)
	if resp := c.post(planURL, url.Values{"state": {string(state)}}); resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("сохранение: код %d", resp.StatusCode)
	}
	resp = c.do("GET", planURL+"/export?format=txt", nil)
	txt, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	for _, want := range []string{
		"Уроки: 1. скидка на скидку: 5, 2. призраки: 10, 3. глубоко невыученные: 15, 4. форель: 20, 5. Варан: 25",
		"Уроки с фиксированной ценой: Пальма: 1, Цао: 5",
		"Месть: Дживан: 60",
		"Тренировки и навыки: кастеты: 0",
		"Фолианты: 1 фолиант: 5, выкуп Тумана: 50",
		"Активация накопителей: горожане (1,05/2, +25% за жизнь), Джон (1/1)",
		"Беру с собой: Туман",
		"Невыученные: —",
		"Остаток: 262,19948 XP",
	} {
		if !strings.Contains(string(txt), want) {
			t.Errorf("в выгрузке нет %q:\n%s", want, txt)
		}
	}
	if strings.Contains(string(txt), "Проверки:") {
		t.Errorf("в плане не должно быть проверок:\n%s", txt)
	}
}

func TestProfilePasswordChange(t *testing.T) {
	c := newClient(t)
	c.post("/register", url.Values{
		"nick": {"archi"}, "discord": {"@archi"}, "password": {"verysecret1"}, "password2": {"verysecret1"},
	})

	code, body := c.getBody("/profile")
	if code != 200 || !strings.Contains(body, "@archi") {
		t.Fatalf("профиль: код %d", code)
	}

	// Неверный текущий пароль — ошибка, пароль не меняется.
	c.post("/profile/password", url.Values{"current": {"нет"}, "password": {"newsecret1"}, "password2": {"newsecret1"}})
	c.post("/logout", nil)
	if resp := c.post("/login", url.Values{"nick": {"archi"}, "password": {"verysecret1"}}); resp.StatusCode != http.StatusSeeOther {
		t.Fatal("старый пароль перестал работать после неудачной смены")
	}
	c.post("/logout", nil)
	if resp := c.post("/login", url.Values{"nick": {"archi"}, "password": {"newsecret1"}}); resp.StatusCode == http.StatusSeeOther {
		t.Fatal("пароль сменился, хотя текущий был указан неверно")
	}

	// Верный текущий пароль — пароль меняется.
	if resp := c.post("/login", url.Values{"nick": {"archi"}, "password": {"verysecret1"}}); resp.StatusCode != http.StatusSeeOther {
		t.Fatal("не вошёл старым паролем")
	}
	if resp := c.post("/profile/password", url.Values{"current": {"verysecret1"}, "password": {"newsecret1"}, "password2": {"newsecret1"}}); resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("смена пароля: код %d", resp.StatusCode)
	}
	c.post("/logout", nil)
	if resp := c.post("/login", url.Values{"nick": {"archi"}, "password": {"verysecret1"}}); resp.StatusCode == http.StatusSeeOther {
		t.Error("старый пароль всё ещё работает")
	}
	if resp := c.post("/login", url.Values{"nick": {"archi"}, "password": {"newsecret1"}}); resp.StatusCode != http.StatusSeeOther {
		t.Error("новый пароль не работает")
	}

	// Дискорд-хендл можно поправить.
	if resp := c.post("/profile/discord", url.Values{"discord": {"@archi#9999"}}); resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("смена дискорда: код %d", resp.StatusCode)
	}
	_, body = c.getBody("/admin")
	if !strings.Contains(body, "@archi#9999") {
		t.Error("новый дискорд-хендл не сохранился")
	}
}

// TestFreshPlanShipsArrays — регрессия: у только что созданного плана все
// списки должны уезжать в конструктор массивами, а не null.
func TestFreshPlanShipsArrays(t *testing.T) {
	c := newClient(t)
	c.post("/register", url.Values{
		"nick": {"archi"}, "discord": {"@archi"}, "password": {"verysecret1"}, "password2": {"verysecret1"},
	})
	resp := c.post("/plans", url.Values{"title": {"Пустой план"}, "balance": {"100"}})
	loc := resp.Header.Get("Location")
	if !strings.HasPrefix(loc, "/plans/") {
		t.Fatalf("план не создался: %s", loc)
	}
	_, body := c.getBody(loc)

	const marker = `id="kt-plan">`
	i := strings.Index(body, marker)
	if i < 0 {
		t.Fatal("в конструкторе нет данных плана")
	}
	rest := body[i+len(marker):]
	j := strings.Index(rest, "</script>")
	if j < 0 {
		t.Fatal("не нашёл конец данных плана")
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(rest[:j]), &got); err != nil {
		t.Fatalf("данные плана не разбираются: %v", err)
	}
	for _, key := range []string{"shop", "picks", "deferred", "not_taken", "ladders", "accumulators", "folios"} {
		if _, ok := got[key].([]any); !ok {
			t.Errorf("%s должен быть массивом, а пришло %#v", key, got[key])
		}
	}
}

// planFromPage достаёт JSON плана, который конструктор получает в kt-plan.
func planFromPage(t *testing.T, body string) map[string]any {
	t.Helper()
	const marker = `id="kt-plan">`
	i := strings.Index(body, marker)
	if i < 0 {
		t.Fatal("на странице нет kt-plan")
	}
	rest := body[i+len(marker):]
	j := strings.Index(rest, "</script>")
	if j < 0 {
		t.Fatal("kt-plan не закрыт")
	}
	var p map[string]any
	if err := json.Unmarshal([]byte(rest[:j]), &p); err != nil {
		t.Fatalf("kt-plan не разобрался: %v", err)
	}
	return p
}

// TestLessonOrderSurvivesSave — порядок уроков расставляет игрок (кнопкой
// «Разложить уроки по требованиям» или стрелками), и сервер сохраняет его как
// есть: ничего не пересортировывает ни при сохранении, ни при отдаче страницы.
func TestLessonOrderSurvivesSave(t *testing.T) {
	c := newClient(t)
	c.post("/register", url.Values{
		"nick": {"order"}, "discord": {"@order"}, "password": {"verysecret1"}, "password2": {"verysecret1"},
	})
	resp := c.post("/plans", url.Values{"title": {"Порядок"}, "preset": {"08-zaskoruzlye-piramidy"}})
	planURL := resp.Header.Get("Location")
	if !strings.HasPrefix(planURL, "/plans/") {
		t.Fatalf("план не создался: %q", planURL)
	}
	code, body := c.getBody(planURL)
	if code != 200 {
		t.Fatalf("конструктор: код %d", code)
	}
	loaded := planFromPage(t, body)

	// Нарочно «неправильный» порядок: «пинг систем» (минимум десятым) идёт
	// вторым, «ученье — свет!» (минимум четвёртым) — третьим.
	// Обязательные позиции каркаса («поцелуй Мелисандры» и заплатка
	// «отключение тренировок») остаются в плане — их место задано текстом.
	loaded["picks"] = []map[string]any{
		{"key": "lesson:poceluy_melisandry"},
		{"key": "patch:otkluchenie_trenirovok"},
		{"key": "lesson:ping_sistem"},
		{"key": "lesson:uchene_svet"},
	}
	delete(loaded, "created_at")
	delete(loaded, "updated_at")
	raw, err := json.Marshal(loaded)
	if err != nil {
		t.Fatal(err)
	}
	if resp := c.post(planURL, url.Values{"state": {string(raw)}}); resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("сохранение: код %d", resp.StatusCode)
	}

	// Порядок вернулся тот же, что сохранили.
	_, body = c.getBody(planURL)
	saved := planFromPage(t, body)
	picks, _ := saved["picks"].([]any)
	var keys []string
	for _, raw := range picks {
		if m, ok := raw.(map[string]any); ok {
			keys = append(keys, m["key"].(string))
		}
	}
	if got := saved["deferred_slots"]; got != float64(2) {
		t.Errorf("слоты невыученных потерялись при сохранении: %v, ждал 2", got)
	}
	want := []string{
		"lesson:poceluy_melisandry", "patch:otkluchenie_trenirovok",
		"lesson:ping_sistem", "lesson:uchene_svet",
	}
	if strings.Join(keys, ",") != strings.Join(want, ",") {
		t.Fatalf("порядок уроков не сохранился: получил %v, ждал %v", keys, want)
	}

	// И в выгрузке уроки идут в этом порядке, с честной нумерацией.
	resp = c.do("GET", planURL+"/export?format=txt", nil)
	txt, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	text := string(txt)
	for _, want := range []string{
		"2. пинг систем: 10",
		"3. ученье: 15",
		"«пинг систем» требует минимум 10-й урок, а стоит 2-м",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("в выгрузке нет %q:\n%s", want, text)
		}
	}
}

// TestStaticAssetsVersioned — ссылки на статику идут с отпечатком содержимого:
// файлы отдаются с часовым кэшем, и без ?v= браузер после выкладки продолжал
// бы запускать старый builder.js (кнопки «не работают», хотя сервер новый).
func TestStaticAssetsVersioned(t *testing.T) {
	c := newClient(t)
	versions := map[string]string{}
	for _, page := range []string{"/", "/login", "/guide"} {
		code, body := c.getBody(page)
		if code != 200 {
			t.Fatalf("%s: код %d", page, code)
		}
		m := assetVersionRE.FindStringSubmatch(body)
		if m == nil {
			t.Fatalf("%s: ссылка на /static/app.css без ?v=:\n%s", page, firstLines(body, 12))
		}
		versions[page] = m[1]
	}
	for page, v := range versions {
		if v != versions["/"] {
			t.Errorf("%s: отпечаток %q, на главной %q — должен быть один на всю сборку", page, v, versions["/"])
		}
	}

	// Страница не кэшируется: иначе после выкладки браузер показал бы старые
	// ссылки на статику, и новые кнопки снова «не работали» бы.
	resp := c.do("GET", "/", nil)
	_, _ = io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	if got := resp.Header.Get("Cache-Control"); got != "no-store" {
		t.Errorf("страницы кэшируются (Cache-Control: %q), ждал no-store", got)
	}

	// Сам файл по-прежнему отдаётся, и это именно текущий конструктор.
	resp = c.do("GET", "/static/builder.js", nil)
	raw, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("builder.js: код %d", resp.StatusCode)
	}
	if !strings.Contains(string(raw), "Разложить уроки по требованиям") {
		t.Error("в builder.js нет кнопки раскладки уроков")
	}
}

var assetVersionRE = regexp.MustCompile(`/static/app\.css\?v=([0-9a-f]+)`)

func firstLines(s string, n int) string {
	lines := strings.Split(s, "\n")
	if len(lines) > n {
		lines = lines[:n]
	}
	return strings.Join(lines, "\n")
}

// TestThemeSwitchInMarkup — переключатель темы есть на каждой странице, тему
// ставит ранний скрипт (до стилей, чтобы не мигало белым), а theme.js подключён
// с тем же отпечатком, что и остальная статика.
func TestThemeSwitchInMarkup(t *testing.T) {
	c := newClient(t)
	for _, page := range []string{"/", "/login", "/guide"} {
		code, body := c.getBody(page)
		if code != 200 {
			t.Fatalf("%s: код %d", page, code)
		}
		for _, want := range []string{
			`id="kt-theme"`,
			`/static/theme.js?v=`,
			`localStorage.getItem('kt-theme')`,
			`prefers-color-scheme: light`,
		} {
			if !strings.Contains(body, want) {
				t.Errorf("%s: нет %q", page, want)
			}
		}
		// Ранний скрипт должен стоять до таблицы стилей.
		script := strings.Index(body, `localStorage.getItem('kt-theme')`)
		css := strings.Index(body, `/static/app.css?v=`)
		if script < 0 || css < 0 || script > css {
			t.Errorf("%s: тема ставится после стилей — страница мигнёт светлым", page)
		}
	}

	// theme.js отдаётся и переключает тему.
	resp := c.do("GET", "/static/theme.js", nil)
	raw, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("theme.js: код %d", resp.StatusCode)
	}
	if !strings.Contains(string(raw), "kt-theme") {
		t.Error("в theme.js нет переключателя темы")
	}
}

// TestPlanSaveJSON — конструктор сохраняет через fetch и ждёт JSON, а не
// редирект: иначе страница уезжала бы на новую загрузку и теряла вкладку.
func TestPlanSaveJSON(t *testing.T) {
	c := newClient(t)
	c.post("/register", url.Values{
		"nick": {"fetchsave"}, "discord": {"@fetchsave"}, "password": {"verysecret1"}, "password2": {"verysecret1"},
	})
	resp := c.post("/plans", url.Values{"title": {"Через fetch"}, "preset": {"09-kontinent-iskonnogo-balansa"}})
	planURL := resp.Header.Get("Location")
	if !strings.HasPrefix(planURL, "/plans/") {
		t.Fatalf("план не создался: %q", planURL)
	}
	_, body := c.getBody(planURL)
	st := planFromPage(t, body)
	st["balance"] = 777
	raw, _ := json.Marshal(st)

	respJSON, out := c.postJSON(planURL, url.Values{"state": {string(raw)}})
	if respJSON.StatusCode != http.StatusOK {
		t.Fatalf("сохранение через fetch: код %d, тело %s", respJSON.StatusCode, out)
	}
	if ct := respJSON.Header.Get("Content-Type"); !strings.Contains(ct, "application/json") {
		t.Errorf("ответ не JSON: %q", ct)
	}
	var got struct {
		OK        bool      `json:"ok"`
		UpdatedAt time.Time `json:"updated_at"`
		URL       string    `json:"url"`
	}
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("ответ не разобрался: %v (%s)", err, out)
	}
	if !got.OK {
		t.Errorf("ok = false: %s", out)
	}
	if got.URL != planURL {
		t.Errorf("url = %q, ждал %q", got.URL, planURL)
	}
	if got.UpdatedAt.IsZero() {
		t.Error("updated_at пустой — клиенту нечем проверять свежесть")
	}

	// План действительно сохранился, а не только ответил ok.
	_, body = c.getBody(planURL)
	if b := planFromPage(t, body)["balance"]; b != float64(777) {
		t.Errorf("баланс после сохранения = %v, ждал 777", b)
	}

	// Без заголовка — прежний редирект: старый путь не сломан.
	if plain := c.post(planURL, url.Values{"state": {string(raw)}}); plain.StatusCode != http.StatusSeeOther {
		t.Errorf("обычная форма: код %d, ждал 303", plain.StatusCode)
	}
}

// fieldStrings вытаскивает значения поля из JSON-массива объектов
// (так в тесте выглядят shop и picks, разобранные из страницы).
func fieldStrings(v any, field string) []string {
	rows, _ := v.([]any)
	out := make([]string, 0, len(rows))
	for _, raw := range rows {
		if m, ok := raw.(map[string]any); ok {
			if s, ok := m[field].(string); ok {
				out = append(out, s)
			}
		}
	}
	return out
}

func hasString(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}

// TestPlanSaveRebasesStaleSkeleton — вкладка, открытая до миграции, не
// откатывает каркас: сервер оставляет свой магазин и каркас, а выбор игрока
// переносит на него. Переименованные позиции находятся по названию,
// исчезнувшие — попадают в lost, чтобы о них можно было сказать вслух.
func TestPlanSaveRebasesStaleSkeleton(t *testing.T) {
	c := newClient(t)
	c.post("/register", url.Values{
		"nick": {"staledraft"}, "discord": {"@staledraft"}, "password": {"verysecret1"}, "password2": {"verysecret1"},
	})
	created := c.post("/plans", url.Values{"title": {"До миграции"}, "preset": {"09-kontinent-iskonnogo-balansa"}})
	planURL := created.Header.Get("Location")
	if !strings.HasPrefix(planURL, "/plans/") {
		t.Fatalf("план не создался: %q", planURL)
	}
	_, body := c.getBody(planURL)
	base, _ := planFromPage(t, body)["updated_at"].(string)
	if base == "" {
		t.Fatal("у плана нет updated_at — черновику не на что опереться")
	}

	// Пока вкладка открыта, каркас обновили. Сохранение с текущей ревизией
	// вправе менять магазин — так выглядят правки каркаса в самом плане.
	migrated := map[string]any{
		"base_updated_at": base,
		"title":           "После миграции",
		"balance":         100.0,
		"skeleton":        "Жизнь 9",
		"shop": []map[string]any{
			{"key": "lesson:keep", "kind": "lesson", "section": "lessons", "title": "остаётся", "cost": 5},
			{"key": "lesson:renamed_new", "kind": "lesson", "section": "lessons", "title": "переименованный", "cost": 20},
		},
		"picks":     []map[string]any{{"key": "lesson:keep"}},
		"deferred":  []map[string]any{},
		"not_taken": []string{},
	}
	raw, _ := json.Marshal(migrated)
	resp, out := c.postJSON(planURL, url.Values{"state": {string(raw)}})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("сохранение миграции: код %d, тело %s", resp.StatusCode, out)
	}
	var first struct {
		OK        bool      `json:"ok"`
		Rebased   bool      `json:"rebased"`
		UpdatedAt time.Time `json:"updated_at"`
	}
	if err := json.Unmarshal([]byte(out), &first); err != nil {
		t.Fatalf("ответ миграции не разобрался: %v (%s)", err, out)
	}
	if first.Rebased {
		t.Errorf("сохранение на текущей ревизии не должно ребейзиться: %s", out)
	}
	if first.UpdatedAt.IsZero() || first.UpdatedAt.UTC().Format(time.RFC3339Nano) == base {
		t.Fatalf("ревизия не сменилась после миграции: %v", first.UpdatedAt)
	}

	// Старая вкладка присылает СВОЙ магазин и свою ревизию. Её магазин
	// не должен вернуться на сервер, а покупки — перенестись на новый.
	stale := map[string]any{
		"base_updated_at": base,
		"title":           "Правки игрока",
		"balance":         555.0,
		"skeleton":        "Жизнь 8",
		"shop": []map[string]any{
			{"key": "lesson:gone", "kind": "lesson", "section": "lessons", "title": "исчезнувший", "cost": 10},
			{"key": "lesson:renamed_old", "kind": "lesson", "section": "lessons", "title": "переименованный", "cost": 1},
			{"key": "lesson:keep", "kind": "lesson", "section": "lessons", "title": "остаётся", "cost": 1},
		},
		"picks": []map[string]any{
			{"key": "lesson:gone"},
			{"key": "lesson:renamed_old"},
			{"key": "lesson:keep"},
		},
		"deferred":  []map[string]any{{"key": "lesson:gone"}},
		"not_taken": []string{"lesson:renamed_old", "lesson:gone"},
	}
	raw, _ = json.Marshal(stale)
	resp, out = c.postJSON(planURL, url.Values{"state": {string(raw)}})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("сохранение старой вкладки: код %d, тело %s", resp.StatusCode, out)
	}
	var got struct {
		OK      bool     `json:"ok"`
		Rebased bool     `json:"rebased"`
		Lost    []string `json:"lost"`
	}
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("ответ не разобрался: %v (%s)", err, out)
	}
	if !got.OK || !got.Rebased {
		t.Fatalf("старая ревизия должна ребейзиться: %s", out)
	}
	if len(got.Lost) != 1 || got.Lost[0] != "исчезнувший" {
		t.Errorf("lost = %v, ждал [исчезнувший]", got.Lost)
	}

	// На сервере остался новый каркас, а правки игрока применились.
	_, body = c.getBody(planURL)
	saved := planFromPage(t, body)
	if saved["skeleton"] != "Жизнь 9" {
		t.Errorf("старый каркас откатил миграцию: %v", saved["skeleton"])
	}
	if saved["title"] != "Правки игрока" {
		t.Errorf("заголовок игрока потерялся: %v", saved["title"])
	}
	if saved["balance"] != float64(555) {
		t.Errorf("баланс игрока потерялся: %v", saved["balance"])
	}
	shop := fieldStrings(saved["shop"], "key")
	if hasString(shop, "lesson:gone") || hasString(shop, "lesson:renamed_old") {
		t.Errorf("магазин старой вкладки вернулся: %v", shop)
	}
	if !hasString(shop, "lesson:keep") || !hasString(shop, "lesson:renamed_new") {
		t.Errorf("новый магазин потерялся: %v", shop)
	}
	picks := fieldStrings(saved["picks"], "key")
	if hasString(picks, "lesson:gone") || hasString(picks, "lesson:renamed_old") {
		t.Errorf("в плане остались старые ключи: %v", picks)
	}
	if !hasString(picks, "lesson:keep") || !hasString(picks, "lesson:renamed_new") {
		t.Errorf("покупки не перенеслись: %v", picks)
	}
	notTaken := fieldStringsAny(saved["not_taken"])
	if hasString(notTaken, "lesson:gone") {
		t.Errorf("«не беру» на исчезнувшую позицию осталось: %v", notTaken)
	}
}

// fieldStringsAny — строковый массив, разобранный из JSON.
func fieldStringsAny(v any) []string {
	rows, _ := v.([]any)
	out := make([]string, 0, len(rows))
	for _, raw := range rows {
		if s, ok := raw.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

// TestPlanSaveWithoutBaseKeepsServerShop — вкладка, загруженная до выкладки,
// не присылает base_updated_at. Доверять её магазину нельзя: иначе она
// откатит миграцию. Игроковые поля при этом сохраняются.
func TestPlanSaveWithoutBaseKeepsServerShop(t *testing.T) {
	c := newClient(t)
	c.post("/register", url.Values{
		"nick": {"legacytab"}, "discord": {"@legacytab"}, "password": {"verysecret1"}, "password2": {"verysecret1"},
	})
	created := c.post("/plans", url.Values{"title": {"Старая вкладка"}, "preset": {"08-zaskoruzlye-piramidy"}})
	planURL := created.Header.Get("Location")
	_, body := c.getBody(planURL)
	before := fieldStrings(planFromPage(t, body)["shop"], "key")
	if len(before) == 0 {
		t.Fatal("у плана пустой магазин")
	}

	stale := map[string]any{
		"title":   "Без ревизии",
		"balance": 321.0,
		"shop": []map[string]any{
			{"key": "lesson:ghost", "kind": "lesson", "section": "lessons", "title": "призрак", "cost": 1},
		},
		"picks":     []map[string]any{{"key": "lesson:ghost"}},
		"deferred":  []map[string]any{},
		"not_taken": []string{},
	}
	raw, _ := json.Marshal(stale)
	resp, out := c.postJSON(planURL, url.Values{"state": {string(raw)}})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("сохранение без ревизии: код %d, тело %s", resp.StatusCode, out)
	}
	var got struct {
		Rebased bool     `json:"rebased"`
		Lost    []string `json:"lost"`
	}
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("ответ не разобрался: %v (%s)", err, out)
	}
	if !got.Rebased {
		t.Errorf("пустая ревизия должна ребейзиться, а не менять каркас: %s", out)
	}
	if len(got.Lost) != 1 || got.Lost[0] != "призрак" {
		t.Errorf("lost = %v, ждал [призрак]", got.Lost)
	}

	_, body = c.getBody(planURL)
	saved := planFromPage(t, body)
	shop := fieldStrings(saved["shop"], "key")
	if hasString(shop, "lesson:ghost") {
		t.Errorf("чужой магазин старой вкладки попал в план: %v", shop)
	}
	if len(shop) != len(before) {
		t.Errorf("магазин изменился: было %d позиций, стало %d", len(before), len(shop))
	}
	if !hasString(shop, "lesson:ping_sistem") {
		t.Errorf("серверный магазин потерялся: %v", shop)
	}
	if picks := fieldStrings(saved["picks"], "key"); hasString(picks, "lesson:ghost") {
		t.Errorf("покупка-призрак осталась в плане: %v", picks)
	}
	if saved["title"] != "Без ревизии" || saved["balance"] != float64(321) {
		t.Errorf("правки игрока не сохранились: title=%v balance=%v", saved["title"], saved["balance"])
	}
}

func TestPlanSaveJSONErrors(t *testing.T) {
	c := newClient(t)

	// Без сессии конструктор получает 401 JSON, а не страницу логина.
	resp, body := c.postJSON("/plans/whatever", url.Values{"state": {"{}"}})
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("без сессии: код %d, тело %s", resp.StatusCode, body)
	}
	if !strings.Contains(body, "сессия") {
		t.Errorf("в ответе нет объяснения: %s", body)
	}

	c.post("/register", url.Values{
		"nick": {"fetcherr"}, "discord": {"@fetcherr"}, "password": {"verysecret1"}, "password2": {"verysecret1"},
	})
	created := c.post("/plans", url.Values{"title": {"Ошибки"}, "preset": {"09-kontinent-iskonnogo-balansa"}})
	planURL := created.Header.Get("Location")

	// Пустое состояние — 400 с текстом, а не 500 и не редирект.
	resp, body = c.postJSON(planURL, url.Values{"state": {""}})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("пустое состояние: код %d, тело %s", resp.StatusCode, body)
	}
	var e struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal([]byte(body), &e); err != nil || e.Error == "" {
		t.Errorf("ошибка не в JSON: %v (%s)", err, body)
	}
}
