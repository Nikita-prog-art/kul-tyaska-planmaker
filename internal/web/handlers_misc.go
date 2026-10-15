package web

import (
	"net/http"
	"strings"

	"kultyaska/internal/auth"
	"kultyaska/internal/plan"
)

// handleGuide — справка по механике: шкалы, цены, правила уроков.
func (s *Server) handleGuide(w http.ResponseWriter, r *http.Request) {
	var users any
	if u := s.currentUser(r); u != nil {
		users = userMap(u)
	}
	ladders := plan.DefaultLadders()
	rows := make([]map[string]any, 0, len(ladders))
	for _, l := range ladders {
		rows = append(rows, map[string]any{
			"key": l.Key, "title": l.Title, "section": plan.SectionTitle(l.Section),
			"from": numStr(&plan.Plan{}, l.From), "to": numStr(&plan.Plan{}, l.To),
			"step": numStr(&plan.Plan{}, l.Step), "note": l.Note,
			"full": trim(&plan.Plan{}, l.Cost.LadderCostUntil(l.From, l.To, l.Step, l.To)),
		})
	}
	s.render(w, r, "guide.jinja", map[string]any{
		"title": "Справка по механике", "nav": "guide",
		"user": users, "ladders": rows, "sections": sectionViews(),
	})
}

// handleAdmin — список пользователей с дискорд-хендлами.
func (s *Server) handleAdmin(w http.ResponseWriter, r *http.Request) {
	users, err := s.st.ListUsers()
	if err != nil {
		http.Error(w, "не удалось загрузить пользователей", http.StatusInternalServerError)
		return
	}
	_, plans, _ := s.st.Stats()
	list := make([]map[string]any, 0, len(users))
	for _, u := range users {
		list = append(list, map[string]any{
			"id": u.ID, "nick": u.Nick, "discord": u.Discord, "is_admin": u.IsAdmin,
			"created": u.CreatedAt.Format("02.01.2006"),
			"last":    u.LastLogin.Format("02.01.2006 15:04"),
			"plans":   s.st.PlansCountByUser(u.ID),
			"temp":    u.TempPassword,
		})
	}
	s.render(w, r, "admin.jinja", map[string]any{
		"title": "Админка", "nav": "admin", "users": list, "total_plans": plans,
	})
}

// handleAdminReset выдаёт пользователю временный пароль.
func (s *Server) handleAdminReset(w http.ResponseWriter, r *http.Request) {
	id := r.FormValue("user_id")
	u, err := s.st.UserByID(id)
	if err != nil {
		http.Error(w, "пользователь не найден", http.StatusNotFound)
		return
	}
	temp := auth.GeneratePassword(12)
	hash, err := auth.Hash(temp)
	if err != nil {
		http.Error(w, "не удалось создать пароль", http.StatusInternalServerError)
		return
	}
	u.PassHash = hash
	u.TempPassword = temp
	if err := s.st.UpdateUser(u); err != nil {
		http.Error(w, "не удалось сохранить пароль", http.StatusInternalServerError)
		return
	}
	if err := s.st.DropSessionsUser(u.ID); err != nil {
		http.Error(w, "не удалось закрыть сессии", http.StatusInternalServerError)
		return
	}
	s.flash(w, "Временный пароль для "+u.Nick+" ("+u.Discord+"): "+temp+" — передайте его в дискорде.")
	http.Redirect(w, r, "/admin", http.StatusSeeOther)
}

// handleAdminToggle выдаёт или снимает права администратора.
func (s *Server) handleAdminToggle(w http.ResponseWriter, r *http.Request) {
	id := r.FormValue("user_id")
	u, err := s.st.UserByID(id)
	if err != nil {
		http.Error(w, "пользователь не найден", http.StatusNotFound)
		return
	}
	me := userFrom(r.Context())
	if u.ID == me.ID && u.IsAdmin {
		s.flash(w, "Нельзя снять права с самого себя.")
		http.Redirect(w, r, "/admin", http.StatusSeeOther)
		return
	}
	u.IsAdmin = !u.IsAdmin
	if err := s.st.UpdateUser(u); err != nil {
		http.Error(w, "не удалось сохранить", http.StatusInternalServerError)
		return
	}
	state := "выданы"
	if !u.IsAdmin {
		state = "сняты"
	}
	s.flash(w, "Права администратора "+state+" для "+u.Nick+".")
	http.Redirect(w, r, "/admin", http.StatusSeeOther)
}

var _ = strings.TrimSpace
