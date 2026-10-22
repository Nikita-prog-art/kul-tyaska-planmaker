// Package web — HTTP-слой приложения: страницы, аутентификация, API планов.
package web

import (
	"crypto/sha256"
	"embed"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/nikolalohinski/gonja/v2"
	"github.com/nikolalohinski/gonja/v2/exec"
	"github.com/nikolalohinski/gonja/v2/loaders"

	"kultyaska/internal/config"
	"kultyaska/internal/store"
)

//go:embed templates/*.jinja static/*
var assets embed.FS

// Version — отображаемая версия сборки.
const Version = "0.1.0"

// Server — приложение.
type Server struct {
	cfg    config.Config
	st     *store.Store
	tpl    map[string]*exec.Template
	dev    bool
	year   string
	assets string // версия статики: сбрасывает кэш браузера после выкладки
}

type ctxKey string

const (
	ctxUser ctxKey = "user"
)

// New создаёт сервер и загружает шаблоны.
func New(cfg config.Config, st *store.Store) (*Server, error) {
	s := &Server{cfg: cfg, st: st, year: fmt.Sprintf("%d", time.Now().Year()), assets: staticVersion()}
	if err := s.loadTemplates(); err != nil {
		return nil, err
	}
	return s, nil
}

// staticVersion — короткий отпечаток встроенной статики. Ссылки в шаблонах
// идут как /static/…?v=<отпечаток>: браузер не залипает на старом builder.js
// после выкладки, хотя файлы отдаются с часовым кэшем.
func staticVersion() string {
	h := sha256.New()
	entries, err := fs.ReadDir(assets, "static")
	if err != nil {
		return Version
	}
	for _, e := range entries { // ReadDir сортирует по имени — отпечаток стабилен
		if e.IsDir() {
			continue
		}
		data, err := fs.ReadFile(assets, "static/"+e.Name())
		if err != nil {
			continue
		}
		_, _ = h.Write([]byte(e.Name()))
		_, _ = h.Write(data)
	}
	return hex.EncodeToString(h.Sum(nil))[:10]
}

func (s *Server) loadTemplates() error {
	loader, err := loaders.NewEmbedFSLoader(".", &assets)
	if err != nil {
		return fmt.Errorf("шаблоны: %w", err)
	}
	entries, err := fs.ReadDir(assets, "templates")
	if err != nil {
		return err
	}
	cfg := gonja.NewConfig()
	cfg.AutoEscape = true // шаблоны экранируют данные, |safe — только для готового HTML
	s.tpl = map[string]*exec.Template{}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".jinja") {
			continue
		}
		tpl, err := exec.NewTemplate("templates/"+e.Name(), cfg, loader, gonja.DefaultEnvironment)
		if err != nil {
			return fmt.Errorf("шаблон %s: %w", e.Name(), err)
		}
		s.tpl[e.Name()] = tpl
	}
	return nil
}

// Handler собирает маршруты.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	static, _ := fs.Sub(assets, "static")
	mux.Handle("GET /static/", http.StripPrefix("/static/", cacheControl(http.FileServer(http.FS(static)))))

	mux.HandleFunc("GET /{$}", s.handleIndex)
	mux.HandleFunc("GET /register", s.handleRegisterForm)
	mux.HandleFunc("POST /register", s.handleRegister)
	mux.HandleFunc("GET /login", s.handleLoginForm)
	mux.HandleFunc("POST /login", s.handleLogin)
	mux.HandleFunc("POST /logout", s.handleLogout)

	mux.HandleFunc("GET /plans", s.auth(s.handlePlans))
	mux.HandleFunc("POST /plans", s.auth(s.handlePlanCreate))
	mux.HandleFunc("GET /plans/{id}", s.auth(s.handlePlanEdit))
	mux.HandleFunc("POST /plans/{id}", s.auth(s.handlePlanSave))
	mux.HandleFunc("POST /plans/{id}/delete", s.auth(s.handlePlanDelete))
	mux.HandleFunc("POST /plans/{id}/duplicate", s.auth(s.handlePlanDuplicate))
	mux.HandleFunc("GET /plans/{id}/export", s.auth(s.handlePlanExport))

	mux.HandleFunc("GET /presets/{name}", s.handlePreset)
	mux.HandleFunc("GET /guide", s.handleGuide)
	mux.HandleFunc("GET /profile", s.auth(s.handleProfile))
	mux.HandleFunc("POST /profile/password", s.auth(s.handleProfilePassword))
	mux.HandleFunc("POST /profile/discord", s.auth(s.handleProfileDiscord))
	mux.HandleFunc("GET /admin", s.auth(s.adminOnly(s.handleAdmin)))
	mux.HandleFunc("POST /admin/reset", s.auth(s.adminOnly(s.handleAdminReset)))
	mux.HandleFunc("POST /admin/toggle", s.auth(s.adminOnly(s.handleAdminToggle)))

	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte("ok"))
	})
	return s.recover(mux)
}

func cacheControl(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "public, max-age=3600")
		h.ServeHTTP(w, r)
	})
}

// --- middleware ---

func (s *Server) recover(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				log.Printf("panic: %v (%s %s)", rec, r.Method, r.URL.Path)
				http.Error(w, "внутренняя ошибка", http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// currentUser достаёт пользователя из сессии.
func (s *Server) currentUser(r *http.Request) *store.User {
	c, err := r.Cookie("kt_session")
	if err != nil || c.Value == "" {
		return nil
	}
	sess, err := s.st.Session(c.Value)
	if err != nil {
		return nil
	}
	u, err := s.st.UserByID(sess.UserID)
	if err != nil {
		return nil
	}
	return u
}

func (s *Server) auth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		u := s.currentUser(r)
		if u == nil {
			http.Redirect(w, r, "/login?next="+r.URL.Path, http.StatusSeeOther)
			return
		}
		next(w, r.WithContext(withUser(r.Context(), u)))
	}
}

func (s *Server) adminOnly(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		u := userFrom(r.Context())
		if u == nil || !u.IsAdmin {
			http.Error(w, "нужны права администратора", http.StatusForbidden)
			return
		}
		next(w, r)
	}
}

// --- рендер ---

func (s *Server) render(w http.ResponseWriter, r *http.Request, name string, data map[string]any) {
	tpl, ok := s.tpl[name]
	if !ok {
		http.Error(w, "шаблон не найден: "+name, http.StatusInternalServerError)
		return
	}
	if data == nil {
		data = map[string]any{}
	}
	u := userFrom(r.Context())
	data["user"] = userMap(u)
	data["version"] = Version
	data["assets"] = s.assets
	data["year"] = s.year
	data["flash"] = s.popFlash(w, r)
	data["path"] = r.URL.Path
	if _, ok := data["title"]; !ok {
		data["title"] = "Культяшка"
	}
	if _, ok := data["error"]; !ok {
		data["error"] = r.URL.Query().Get("error")
	}
	ctx := exec.NewContext(data)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	// Страницы личные и меняются от выкладки к выкладке: без no-store браузер
	// мог бы показать старую вёрстку со ссылками на старую статику.
	w.Header().Set("Cache-Control", "no-store")
	if err := tpl.Execute(w, ctx); err != nil {
		log.Printf("шаблон %s: %v", name, err)
	}
}

func userMap(u *store.User) map[string]any {
	if u == nil {
		return nil
	}
	return map[string]any{
		"id": u.ID, "nick": u.Nick, "discord": u.Discord, "is_admin": u.IsAdmin,
		"created_at": u.CreatedAt.Format("02.01.2006"),
	}
}

// flash ставит одноразовое сообщение в cookie.
func (s *Server) flash(w http.ResponseWriter, text string) {
	http.SetCookie(w, &http.Cookie{
		Name: "kt_flash", Value: encodeCookie(text), Path: "/", MaxAge: 60,
		HttpOnly: true, SameSite: http.SameSiteLaxMode, Secure: s.cfg.SecureCookies,
	})
}

func (s *Server) popFlash(w http.ResponseWriter, r *http.Request) string {
	c, err := r.Cookie("kt_flash")
	if err != nil || c.Value == "" {
		return ""
	}
	http.SetCookie(w, &http.Cookie{
		Name: "kt_flash", Value: "", Path: "/", MaxAge: -1,
		HttpOnly: true, SameSite: http.SameSiteLaxMode, Secure: s.cfg.SecureCookies,
	})
	return decodeCookie(c.Value)
}

func (s *Server) setSession(w http.ResponseWriter, token string, exp time.Time) {
	http.SetCookie(w, &http.Cookie{
		Name: "kt_session", Value: token, Path: "/", Expires: exp,
		HttpOnly: true, SameSite: http.SameSiteLaxMode, Secure: s.cfg.SecureCookies,
	})
}

// --- утилиты ---

func mustJSON(v any) string {
	raw, err := json.Marshal(v)
	if err != nil {
		return "null"
	}
	return string(raw)
}

// jsonForHTML безопасно встраивает JSON в <script type="application/json">.
// В шаблоне значение вставляется с фильтром |safe.
func jsonForHTML(v any) string {
	raw, err := json.Marshal(v)
	if err != nil {
		return "null"
	}
	esc := strings.NewReplacer("<", "\\u003c", ">", "\\u003e", "&", "\\u0026", "\u2028", "\\u2028", "\u2029", "\\u2029")
	return esc.Replace(string(raw))
}

// encodeCookie упаковывает текст сообщения в ASCII (куки не терпят кириллицу).
func encodeCookie(s string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(s))
}

func decodeCookie(s string) string {
	raw, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return ""
	}
	return string(raw)
}
