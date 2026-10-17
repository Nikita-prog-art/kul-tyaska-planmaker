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

// AccumulatorValues — состояние накопителя на момент перерождения с учётом
// уже сделанных покупок: заряды, максимум и скорость зарядки. Заряд считается
// целыми единицами: 0,8 — это ещё не заряд, а 1 из 2 — уже заряд.
//
// Прибавка за прожитую жизнь (rate) сюда не входит: эти проценты накопитель
// получит в следующей жизни, а на перерождении тратится то, что накоплено.
func (p *Plan) AccumulatorValues(a *Accumulator) (charges, max, rate float64) {
	charges, max, rate = a.Charges, a.Max, a.Rate
	for _, pick := range p.Picks {
		it := p.Item(pick.Key)
		if it == nil {
			continue
		}
		times := pick.Qty
		if times < 1 {
			times = 1
		}
		for _, e := range it.Effects {
			if e.Accumulator != a.Key {
				continue
			}
			charges += e.Charges * float64(times)
			max += e.Max * float64(times)
			// Скорость зарядки покупка задаёт, а не прибавляет: «до 25% за жизнь».
			if e.Rate > 0 {
				rate = e.Rate
			}
		}
	}
	return charges, max, rate
}

// accumulatorsTouched — покупки, которые вообще влияют на этот накопитель.
func (p *Plan) accumulatorsTouched(a *Accumulator) bool {
	for _, pick := range p.Picks {
		it := p.Item(pick.Key)
		if it == nil {
			continue
		}
		for _, e := range it.Effects {
			if e.Accumulator == a.Key {
				return true
			}
		}
	}
	return false
}

// AccumulatorKnown сообщает, известны ли про накопитель заряды: либо каркас
// их указал, либо их меняют покупки. Если про заряды в тексте ничего нет
// (так бывает у накопителей, механика которых не расписана), проверять нечего.
func (p *Plan) AccumulatorKnown(a *Accumulator) bool {
	return a.Charges != 0 || a.Rate != 0 || p.accumulatorsTouched(a)
}

// AccumulatorState говорит, можно ли активировать накопитель: он должен быть
// куплен (если каркас этого требует) и иметь хотя бы один целый заряд.
// Накопитель на 1 из 2 уже можно тратить — заряд тратится по одному.
func (p *Plan) AccumulatorState(a *Accumulator) AttachmentState {
	if missing := p.missingPrereqs(a.Requires); len(missing) > 0 {
		return AttachmentState{Reason: "сначала купи " + p.prereqList(missing)}
	}
	charges, _, _ := p.AccumulatorValues(a)
	if p.AccumulatorKnown(a) && charges < 1-1e-9 {
		return AttachmentState{Reason: "нет целого заряда: " + comma(charges, 5, true)}
	}
	return AttachmentState{Ready: true}
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
