package web

import (
	"net/http"
	"strings"
	"unicode/utf8"

	"kultyaska/internal/auth"
)

func (s *Server) handleProfile(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r.Context())
	plans := s.st.PlansCountByUser(u.ID)
	s.render(w, r, "profile.jinja", map[string]any{
		"title": "Профиль", "nav": "profile",
		"profile": map[string]any{
			"nick": u.Nick, "discord": u.Discord, "is_admin": u.IsAdmin,
			"created": u.CreatedAt.Format("02.01.2006"), "plans": plans,
			"temp": u.TempPassword != "",
		},
	})
}

func (s *Server) handleProfilePassword(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r.Context())
	cur := r.FormValue("current")
	pass := r.FormValue("password")
	pass2 := r.FormValue("password2")

	fail := func(msg string) {
		s.render(w, r, "profile.jinja", map[string]any{
			"title": "Профиль", "nav": "profile", "error": msg,
			"profile": map[string]any{
				"nick": u.Nick, "discord": u.Discord, "is_admin": u.IsAdmin,
				"created": u.CreatedAt.Format("02.01.2006"),
				"plans":   s.st.PlansCountByUser(u.ID), "temp": u.TempPassword != "",
			},
		})
	}
	switch {
	case !auth.Check(u.PassHash, cur):
		fail("Текущий пароль не подошёл.")
		return
	case utf8.RuneCountInString(pass) < 8:
		fail("Новый пароль должен быть не короче 8 символов.")
		return
	case pass != pass2:
		fail("Новые пароли не совпадают.")
		return
	case pass == cur:
		fail("Новый пароль совпадает с текущим.")
		return
	}
	hash, err := auth.Hash(pass)
	if err != nil {
		fail("Не удалось обработать пароль.")
		return
	}
	u.PassHash = hash
	u.TempPassword = ""
	if err := s.st.UpdateUser(u); err != nil {
		http.Error(w, "не удалось сохранить пароль", http.StatusInternalServerError)
		return
	}
	s.flash(w, "Пароль обновлён.")
	http.Redirect(w, r, "/profile", http.StatusSeeOther)
}

// handleProfileDiscord позволяет поправить дискорд-хендл, если игрок ошибся при регистрации.
func (s *Server) handleProfileDiscord(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r.Context())
	d := strings.TrimSpace(r.FormValue("discord"))
	if d == "" {
		s.flash(w, "Дискорд-хендл не может быть пустым.")
		http.Redirect(w, r, "/profile", http.StatusSeeOther)
		return
	}
	u.Discord = d
	if err := s.st.UpdateUser(u); err != nil {
		http.Error(w, "не удалось сохранить", http.StatusInternalServerError)
		return
	}
	s.flash(w, "Дискорд-хендл обновлён: "+d)
	http.Redirect(w, r, "/profile", http.StatusSeeOther)
}
