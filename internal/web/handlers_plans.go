package web

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"kultyaska/internal/plan"
	"kultyaska/internal/store"
)

// planFor достаёт план пользователя или пишет ошибку.
func (s *Server) planFor(w http.ResponseWriter, r *http.Request) *plan.Plan {
	u := userFrom(r.Context())
	p, err := s.st.Plan(r.PathValue("id"))
	if err != nil || p.UserID != u.ID {
		s.saveError(w, r, http.StatusNotFound, "план не найден")
		return nil
	}
	return p
}

func (s *Server) handlePlans(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r.Context())
	plans, err := s.st.PlansByUser(u.ID)
	if err != nil {
		http.Error(w, "не удалось загрузить планы", http.StatusInternalServerError)
		return
	}
	list := make([]map[string]any, 0, len(plans))
	for _, p := range plans {
		sum := plan.Compute(p)
		list = append(list, map[string]any{
			"id": p.ID, "title": p.Title, "emoji": p.Emoji,
			"balance": numStr(p, p.Balance), "spent": trim(p, sum.Total),
			"remainder": trim(p, sum.Remainder), "picks": len(p.Picks),
			"lessons": countLessons(p), "updated": p.UpdatedAt.Format("02.01.2006 15:04"),
			"deferred": len(p.Deferred), "skeleton": p.Skeleton,
			"warning": len(sum.Warnings) > 0,
		})
	}
	s.render(w, r, "plans.jinja", map[string]any{
		"title": "Мои планы", "nav": "plans", "plans": list,
		"presets":      presetViews(s.presetList()),
		"presets_json": jsonForHTML(s.presetList()),
	})
}

func countLessons(p *plan.Plan) int {
	n := 0
	for _, pk := range p.Picks {
		if it := p.Item(pk.Key); it != nil && it.Kind == plan.KindLesson && !it.Fixed {
			n++
		}
	}
	return n
}

func (s *Server) handlePlanCreate(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r.Context())
	title := strings.TrimSpace(r.FormValue("title"))
	emoji := strings.TrimSpace(r.FormValue("emoji"))

	var p *plan.Plan
	if name := strings.TrimSpace(r.FormValue("preset")); name != "" {
		sk, err := s.preset(name)
		if err != nil {
			s.flash(w, "Каркас «"+name+"» не найден.")
			http.Redirect(w, r, "/plans", http.StatusSeeOther)
			return
		}
		p = sk.NewPlan(u.ID, title, emoji)
	} else {
		if title == "" {
			title = "Новый план"
		}
		p = plan.EmptyPlan(u.ID, title, emoji, parseFloat(r.FormValue("balance"), 0))
	}
	if err := s.st.PutPlan(p); err != nil {
		http.Error(w, "не удалось сохранить план", http.StatusInternalServerError)
		return
	}
	s.flash(w, "План создан. Магазин можно загрузить каркасом посмертия.")
	http.Redirect(w, r, "/plans/"+p.ID, http.StatusSeeOther)
}

func (s *Server) handlePlanEdit(w http.ResponseWriter, r *http.Request) {
	p := s.planFor(w, r)
	if p == nil {
		return
	}
	sum := plan.Compute(p)
	s.render(w, r, "plan.jinja", map[string]any{
		"title":        p.Title,
		"nav":          "plans",
		"plan":         planView(p),
		"plan_json":    jsonForHTML(p),
		"sections":     jsonForHTML(plan.Sections),
		"summary":      summaryView(sum),
		"presets":      presetViews(s.presetList()),
		"presets_json": jsonForHTML(s.presetList()),
		"export_text":  plan.Render(p, sum),
	})
}

// saveState — состояние редактора, которое присылает браузер.
type saveState struct {
	Title        string             `json:"title"`
	Emoji        string             `json:"emoji"`
	Balance      float64            `json:"balance"`
	Notes        string             `json:"notes"`
	Decimals     bool               `json:"decimals"`
	Skeleton     string             `json:"skeleton"`
	Earned       float64            `json:"earned"`
	Mults        []string           `json:"multipliers"`
	Source       string             `json:"source"`
	Shop         []plan.ShopItem    `json:"shop"`
	Picks        []plan.Pick        `json:"picks"`
	Deferred     []plan.Deferred    `json:"deferred"`
	NotTaken     []string           `json:"not_taken"`
	Ladders      []plan.LadderCfg   `json:"ladders"`
	Accumulators []plan.Accumulator `json:"accumulators"`
	Folios       []plan.Folio       `json:"folios"`
	Requirements []plan.Requirement `json:"requirements"`
	// DeferredSlots — сколько слотов «невыученных» открыто в каркасе.
	DeferredSlots int `json:"deferred_slots"`
	// BaseUpdatedAt — ревизия плана, на которой собран черновик.
	// Пустое значение означает старую вкладку: её магазин не авторитетен.
	BaseUpdatedAt string `json:"base_updated_at"`
}

// wantsJSON — сохранение пришло из конструктора через fetch, а не из формы.
// Таким запросам отвечаем JSON-ом, чтобы страница не уезжала на редирект.
func wantsJSON(r *http.Request) bool {
	return r.Header.Get("X-KT-Save") == "fetch"
}

// saveError отвечает ошибкой в том формате, который ждёт клиент.
func (s *Server) saveError(w http.ResponseWriter, r *http.Request, status int, msg string) {
	if !wantsJSON(r) {
		http.Error(w, msg, status)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

func (s *Server) handlePlanSave(w http.ResponseWriter, r *http.Request) {
	p := s.planFor(w, r)
	if p == nil {
		return
	}
	raw := r.FormValue("state")
	if raw == "" {
		s.saveError(w, r, http.StatusBadRequest, "нет данных плана")
		return
	}
	var st saveState
	if err := json.Unmarshal([]byte(raw), &st); err != nil {
		s.saveError(w, r, http.StatusBadRequest, "не разобрал состояние плана: "+err.Error())
		return
	}
	// Ревизия, на которой сделан черновик. Если она отстала от сервера
	// (миграция каркаса, вторая вкладка, правка админом), магазин и каркас
	// из запроса не применяем: иначе открытая вкладка откатит миграцию.
	// Старые вкладки, которые ещё не знают про base_updated_at, тоже
	// попадают в rebase — это безопаснее, чем доверять их копии каркаса.
	base := strings.TrimSpace(st.BaseUpdatedAt)
	if base == "" {
		base = strings.TrimSpace(r.FormValue("base_updated_at"))
	}
	rebased := !sameRevision(base, p.UpdatedAt)
	var lost []string
	if rebased {
		lost = rebaseEditorState(p, &st)
	} else {
		applyEditorState(p, &st)
	}
	// Обязательные позиции каркаса доставляются в план (см. plan.Normalize).
	p.Normalize()
	if err := s.st.PutPlan(p); err != nil {
		s.saveError(w, r, http.StatusInternalServerError, "не удалось сохранить план")
		return
	}
	if wantsJSON(r) {
		if lost == nil {
			lost = []string{}
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":         true,
			"updated_at": p.UpdatedAt,
			"url":        "/plans/" + p.ID,
			"rebased":    rebased,
			"lost":       lost,
		})
		return
	}
	if r.FormValue("export") == "1" {
		http.Redirect(w, r, "/plans/"+p.ID+"/export", http.StatusSeeOther)
		return
	}
	if rebased {
		msg := "Каркас обновился — план перенесён на серверную версию."
		if len(lost) > 0 {
			msg += fmt.Sprintf(" Потеряно позиций: %d.", len(lost))
		}
		s.flash(w, msg)
	} else {
		s.flash(w, "План сохранён.")
	}
	http.Redirect(w, r, "/plans/"+p.ID, http.StatusSeeOther)
}

func normalizePicks(p *plan.Plan, picks []plan.Pick) []plan.Pick {
	out := make([]plan.Pick, 0, len(picks))
	for _, pk := range picks {
		if p.Item(pk.Key) == nil {
			continue
		}
		if pk.Qty <= 0 {
			pk.Qty = 1
		}
		out = append(out, pk)
	}
	return out
}

func normalizeDeferred(p *plan.Plan, def []plan.Deferred) []plan.Deferred {
	out := make([]plan.Deferred, 0, len(def))
	for i, d := range def {
		if p.Item(d.Key) == nil {
			continue
		}
		if d.Slot <= 0 {
			d.Slot = i + 1
		}
		out = append(out, d)
	}
	return out
}

func normalizeKeys(p *plan.Plan, keys []string) []string {
	out := make([]string, 0, len(keys))
	for _, k := range keys {
		if p.Item(k) != nil {
			out = append(out, k)
		}
	}
	return out
}

func (s *Server) handlePlanDelete(w http.ResponseWriter, r *http.Request) {
	p := s.planFor(w, r)
	if p == nil {
		return
	}
	if err := s.st.DeletePlan(p); err != nil {
		http.Error(w, "не удалось удалить план", http.StatusInternalServerError)
		return
	}
	s.flash(w, "План «"+p.Title+"» удалён.")
	http.Redirect(w, r, "/plans", http.StatusSeeOther)
}

func (s *Server) handlePlanDuplicate(w http.ResponseWriter, r *http.Request) {
	p := s.planFor(w, r)
	if p == nil {
		return
	}
	dup := *p
	dup.ID = store.NewID()
	dup.Title = p.Title + " (копия)"
	if err := s.st.PutPlan(&dup); err != nil {
		http.Error(w, "не удалось скопировать план", http.StatusInternalServerError)
		return
	}
	s.flash(w, "Создана копия плана.")
	http.Redirect(w, r, "/plans/"+dup.ID, http.StatusSeeOther)
}

func (s *Server) handlePlanExport(w http.ResponseWriter, r *http.Request) {
	p := s.planFor(w, r)
	if p == nil {
		return
	}
	sum := plan.Compute(p)
	text := plan.Render(p, sum)
	if r.URL.Query().Get("format") == "txt" {
		name := "plan-" + plan.Slug(p.Title) + ".txt"
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", name))
		_, _ = w.Write([]byte(text))
		return
	}
	s.render(w, r, "export.jinja", map[string]any{
		"title": "Экспорт: " + p.Title, "nav": "plans",
		"plan": planView(p), "summary": summaryView(sum), "export_text": text,
		"total": trim(p, sum.Total), "remainder": trim(p, sum.Remainder),
		"lessons": countLessons(p),
	})
}

// --- служебное ---

func parseFloat(s string, def float64) float64 {
	s = strings.TrimSpace(strings.ReplaceAll(s, ",", "."))
	if s == "" {
		return def
	}
	var v float64
	if _, err := fmt.Sscanf(s, "%g", &v); err != nil {
		return def
	}
	return v
}
