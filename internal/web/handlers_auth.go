package web

import (
	"net/http"
	"net/url"
	"strings"
	"unicode/utf8"

	"kultyaska/internal/auth"
	"kultyaska/internal/store"
)

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	if u := s.currentUser(r); u != nil {
		http.Redirect(w, r, "/plans", http.StatusSeeOther)
		return
	}
	s.render(w, r, "index.jinja", map[string]any{
		"title": "Культяшка — планировщик перерождений",
		"nav":   "index",
	})
}

func (s *Server) handleRegisterForm(w http.ResponseWriter, r *http.Request) {
	if u := s.currentUser(r); u != nil {
		http.Redirect(w, r, "/plans", http.StatusSeeOther)
		return
	}
	s.render(w, r, "register.jinja", map[string]any{
		"title": "Регистрация",
		"nav":   "register",
	})
}

func (s *Server) handleRegister(w http.ResponseWriter, r *http.Request) {
	if u := s.currentUser(r); u != nil {
		http.Redirect(w, r, "/plans", http.StatusSeeOther)
		return
	}
	nick := strings.TrimSpace(r.FormValue("nick"))
	discord := strings.TrimSpace(r.FormValue("discord"))
	pass := r.FormValue("password")
	pass2 := r.FormValue("password2")

	fail := func(msg string) {
		s.render(w, r, "register.jinja", map[string]any{
			"title": "Регистрация", "nav": "register",
			"error": msg, "nick": nick, "discord": discord,
		})
	}
	switch {
	case utf8.RuneCountInString(nick) < 3 || utf8.RuneCountInString(nick) > 24:
		fail("Ник должен быть от 3 до 24 символов.")
		return
	case strings.ContainsAny(nick, " \t/\\<>\"'&"):
		fail("В нике допустимы буквы, цифры, «_», «-», «.», «@».")
		return
	case discord == "":
		fail("Укажите дискорд-хендл: по нему я выдам доступ, если вы забудете пароль.")
		return
	case utf8.RuneCountInString(pass) < 8:
		fail("Пароль должен быть не короче 8 символов.")
		return
	case pass != pass2:
		fail("Пароли не совпадают.")
		return
	}
	hash, err := auth.Hash(pass)
	if err != nil {
		fail("Не удалось обработать пароль.")
		return
	}
	count, _ := s.st.CountUsers()
	u := &store.User{
		Nick:     nick,
		Discord:  discord,
		PassHash: hash,
		IsAdmin:  count == 0 || s.cfg.IsAdmin(nick),
	}
	if err := s.st.CreateUser(u); err != nil {
		fail("Такой ник уже занят.")
		return
	}
	sess, err := s.st.CreateSession(u.ID, s.cfg.SessionTTL)
	if err != nil {
		fail("Не удалось создать сессию.")
		return
	}
	s.setSession(w, sess.Token, sess.Expires)
	s.flash(w, "Добро пожаловать, "+u.Nick+"! Первым делом создайте план.")
	http.Redirect(w, r, "/plans", http.StatusSeeOther)
}

func (s *Server) handleLoginForm(w http.ResponseWriter, r *http.Request) {
	if u := s.currentUser(r); u != nil {
		http.Redirect(w, r, "/plans", http.StatusSeeOther)
		return
	}
	s.render(w, r, "login.jinja", map[string]any{
		"title": "Вход", "nav": "login",
		"next": safeNext(r.URL.Query().Get("next")),
	})
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	nick := strings.TrimSpace(r.FormValue("nick"))
	pass := r.FormValue("password")
	next := safeNext(r.FormValue("next"))
	u, err := s.st.UserByNick(nick)
	if err != nil || !auth.Check(u.PassHash, pass) {
		s.render(w, r, "login.jinja", map[string]any{
			"title": "Вход", "nav": "login", "nick": nick, "next": next,
			"error": "Неверный ник или пароль.",
		})
		return
	}
	sess, err := s.st.CreateSession(u.ID, s.cfg.SessionTTL)
	if err != nil {
		http.Error(w, "не удалось создать сессию", http.StatusInternalServerError)
		return
	}
	s.setSession(w, sess.Token, sess.Expires)
	if next == "" {
		next = "/plans"
	}
	http.Redirect(w, r, next, http.StatusSeeOther)
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie("kt_session"); err == nil && c.Value != "" {
		_ = s.st.DeleteSession(c.Value)
	}
	http.SetCookie(w, &http.Cookie{
		Name: "kt_session", Value: "", Path: "/", MaxAge: -1,
		HttpOnly: true, SameSite: http.SameSiteLaxMode, Secure: s.cfg.SecureCookies,
	})
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

// safeNext не даёт увести пользователя на чужой сайт после входа.
func safeNext(next string) string {
	if next == "" || !strings.HasPrefix(next, "/") || strings.HasPrefix(next, "//") {
		return ""
	}
	if u, err := url.Parse(next); err != nil || u.Host != "" {
		return ""
	}
	return next
}
