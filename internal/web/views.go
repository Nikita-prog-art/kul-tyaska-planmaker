package web

import (
	"fmt"
	"strings"

	"kultyaska/internal/plan"
)

// trim печатает стоимость плана.
func trim(p *plan.Plan, v float64) string {
	if !p.Options.Decimals {
		return strconvI(v)
	}
	return commaStr(v)
}

// numStr печатает значение шкалы.
func numStr(p *plan.Plan, v float64) string {
	if v == float64(int64(v)) {
		return strconvI(v)
	}
	return commaStr(v)
}

func strconvI(v float64) string {
	neg := v < 0
	if neg {
		v = -v
	}
	s := itoa(int64(v + 0.5))
	if neg {
		return "-" + s
	}
	return s
}

func itoa(n int64) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}

func commaStr(v float64) string {
	s := fmt.Sprintf("%.5f", v)
	if strings.Contains(s, ".") {
		s = strings.TrimRight(s, "0")
		s = strings.TrimRight(s, ".")
	}
	return strings.Replace(s, ".", ",", 1)
}

// planView — план для шаблонов (ключи в нижнем регистре, как принято в Jinja).
func planView(p *plan.Plan) map[string]any {
	return map[string]any{
		"id": p.ID, "title": p.Title, "emoji": p.Emoji, "balance": p.Balance,
		"skeleton": p.Skeleton, "earned": p.Earned, "notes": p.Notes,
	}
}

// summaryView — расчёт плана для шаблонов.
func summaryView(s *plan.Summary) map[string]any {
	future := make([]map[string]any, 0, len(s.Future))
	for _, f := range s.Future {
		future = append(future, map[string]any{"text": f.Text, "met": f.Met})
	}
	return map[string]any{
		"sections": s.Sections, "warnings": s.Warnings, "total": s.Total,
		"remainder": s.Remainder, "balance": s.Balance,
		"future": future, "rewards": s.Rewards,
	}
}

// presetViews — каркасы для шаблонов.
func presetViews(list []PresetInfo) []map[string]any {
	out := make([]map[string]any, 0, len(list))
	for _, p := range list {
		out = append(out, map[string]any{
			"name": p.Name, "file": p.File, "balance": p.Balance, "earned": p.Earned,
			"lessons": p.Lessons, "items": p.Items, "note": p.Note,
			"multipliers": p.Multipliers,
		})
	}
	return out
}

// sectionViews — секции плана для шаблонов.
func sectionViews() []map[string]any {
	out := make([]map[string]any, 0, len(plan.Sections))
	for _, s := range plan.Sections {
		out = append(out, map[string]any{"key": s.Key, "title": s.Title, "hint": s.Hint})
	}
	return out
}
