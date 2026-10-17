package plan

import (
	"fmt"
	"strings"
)

// AttachmentState — можно ли отмечать накопитель или фолиант в плане.
// Отметки, которые план позволить не может, конструктор не даёт поставить,
// а Normalize снимает: так сохранённый план всегда сходится сам с собой.
type AttachmentState struct {
	Ready  bool   `json:"ready"`
	Reason string `json:"reason,omitempty"` // почему нельзя — подсказка в панели
}

// AccumulatorCharges — заряды накопителя на момент перерождения: к тому, что
// записано в каркасе, добавляется прирост за прожитую жизнь. Если про заряды
// в тексте ничего не известно (нули в каркасе), третий результат — false.
func (p *Plan) AccumulatorCharges(a *Accumulator) (charges, max float64, known bool) {
	if a.Charges == 0 && a.Rate == 0 {
		return 0, a.Max, false
	}
	return a.Charges + a.Rate/100, a.Max, true
}

// AccumulatorState говорит, можно ли активировать накопитель: купленный
// (если каркас этого требует) и полностью заряженный.
func (p *Plan) AccumulatorState(a *Accumulator) AttachmentState {
	if missing := p.missingPrereqs(a.Requires); len(missing) > 0 {
		return AttachmentState{Reason: "сначала купи " + p.prereqList(missing)}
	}
	charges, max, known := p.AccumulatorCharges(a)
	if known && charges+1e-9 < max {
		return AttachmentState{Reason: fmt.Sprintf("заряд не полный: %s из %s%s",
			formatNum(charges), formatNum(max), rateSuffix(a.Rate))}
	}
	return AttachmentState{Ready: true}
}

// rateSuffix поясняет, что заряд посчитан с приростом за прожитую жизнь.
func rateSuffix(rate float64) string {
	if rate == 0 {
		return ""
	}
	return fmt.Sprintf(" (с +%s%% за жизнь)", formatNum(rate))
}

// FolioState говорит, можно ли взять фолиант с собой: сначала урок метода,
// потом выкуп. Сам выбор фолианта бесплатный — он оплачен этими покупками.
func (p *Plan) FolioState(f *Folio) AttachmentState {
	missing := p.missingPrereqs(f.Requires)
	if len(missing) == 0 {
		return AttachmentState{Ready: true}
	}
	return AttachmentState{Reason: p.folioReason(missing)}
}

// folioReason объясняет, чего не хватает для фолианта: урок метода, выкуп
// или всё сразу.
func (p *Plan) folioReason(missing []string) string {
	var lessons, buys []string
	for _, key := range missing {
		if it := p.Item(key); it != nil && it.Kind == KindLesson {
			lessons = append(lessons, "«"+displayTitle(it)+"»")
			continue
		}
		buys = append(buys, p.prereqText(key))
	}
	var parts []string
	if len(lessons) > 0 {
		parts = append(parts, "сначала урок "+strings.Join(lessons, ", "))
	}
	if len(buys) > 0 {
		parts = append(parts, "нужен выкуп "+strings.Join(buys, ", "))
	}
	return strings.Join(parts, ", ")
}

// missingPrereqs перечисляет ключи из requires, которых в плане нет.
// Отложенный урок считается взятым: он уезжает в следующую жизнь.
func (p *Plan) missingPrereqs(keys []string) []string {
	var out []string
	for _, key := range keys {
		if key == "" || p.Picked(key) || p.IsDeferred(key) {
			continue
		}
		out = append(out, key)
	}
	return out
}

// prereqList печатает список покупок для подсказки.
func (p *Plan) prereqList(keys []string) string {
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, p.prereqText(key))
	}
	return strings.Join(parts, ", ")
}

// prereqText — «название (цена)» для подсказки о недостающей покупке.
// У выкупа фолианта убираем глагол из названия: «нужен выкуп «метод Тумана»».
func (p *Plan) prereqText(key string) string {
	it := p.Item(key)
	if it == nil {
		return key
	}
	title := strings.TrimSpace(strings.TrimPrefix(displayTitle(it), "выкупить"))
	if title == "" {
		title = displayTitle(it)
	}
	title = quoteTitle(title)
	if it.Cost > 0 {
		return fmt.Sprintf("%s (%s XP)", title, formatNum(it.Cost))
	}
	return title
}

// quoteTitle берёт название в кавычки, если своих кавычек в нём нет.
func quoteTitle(s string) string {
	if strings.ContainsAny(s, "«»") {
		return s
	}
	return "«" + s + "»"
}

// liveKeys оставляет только те ключи, которые есть в магазине плана.
func (p *Plan) liveKeys(keys []string) []string {
	seen := map[string]bool{}
	for i := range p.Shop {
		seen[p.Shop[i].Key] = true
	}
	return existKeys(seen, keys)
}

// pruneAttachments снимает отметки, которые план позволить не может:
// незаряженный накопитель и фолиант без урока и выкупа.
func (p *Plan) pruneAttachments() {
	for i := range p.Accumulators {
		a := &p.Accumulators[i]
		if a.Activate && !p.AccumulatorState(a).Ready {
			a.Activate = false
		}
	}
	for i := range p.Folios {
		f := &p.Folios[i]
		if f.Take && !p.FolioState(f).Ready {
			f.Take = false
		}
	}
}
