package web_test

import (
	"encoding/json"
	"io"
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

	// Сохранение состояния и проверка экспорта.
	state := map[string]any{
		"title": "Праведность без контроля пола", "emoji": "🩸", "balance": 1605.24148,
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
		"3. ученье — свет!: 15",
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
