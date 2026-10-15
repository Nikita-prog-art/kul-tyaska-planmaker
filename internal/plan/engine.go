package plan

import (
	"fmt"
	"sort"
	"strings"
)

// Line — строка плана внутри секции.
type Line struct {
	Key      string  `json:"key"`
	Title    string  `json:"title"`
	Cost     float64 `json:"cost"`
	Number   int     `json:"number,omitempty"` // номер урока
	Fixed    bool    `json:"fixed,omitempty"`
	Ladder   bool    `json:"ladder,omitempty"`
	Qty      int     `json:"qty,omitempty"`
	Note     string  `json:"note,omitempty"`
	Deferred bool    `json:"deferred,omitempty"`
	NotTaken bool    `json:"not_taken,omitempty"`
}

// SectionSummary — итог по секции.
type SectionSummary struct {
	Key       string  `json:"key"`
	Title     string  `json:"title"`
	Hint      string  `json:"hint,omitempty"`
	Order     int     `json:"order"`
	Lines     []Line  `json:"lines"`
	Sum       float64 `json:"sum"`
	Remainder float64 `json:"remainder"`
}

// Summary — полный расчёт плана.
type Summary struct {
	Sections      []SectionSummary `json:"sections"`
	Total         float64          `json:"total"`
	Remainder     float64          `json:"remainder"`
	Balance       float64          `json:"balance"`
	Warnings      []string         `json:"warnings"`
	DeferredLimit int              `json:"deferred_limit"`
	DeferredUsed  int              `json:"deferred_used"`
	NotTaken      []Line           `json:"not_taken_lines"`
}

// LessonOrdinal — номер урока по ключу позиции.
type LessonOrdinal map[string]int

// Compute считает план: стоимость покупок, суммы по секциям и остаток.
func Compute(p *Plan) *Summary {
	lines := map[string][]Line{}
	ordinals := LessonOrdinal{}
	var warnings []string
	total := 0.0

	// Уроки: порядок покупки задаёт нумерацию и цену.
	paidIdx := 0
	for _, pick := range p.Picks {
		item := p.Item(pick.Key)
		if item == nil {
			continue
		}
		if item.Kind != KindLesson {
			continue
		}
		if p.IsDeferred(pick.Key) {
			continue
		}
		l := Line{Key: item.Key, Title: displayTitle(item), Fixed: item.Fixed}
		if item.Fixed {
			l.Cost = item.Cost
		} else {
			paidIdx++
			l.Number = paidIdx
			ordinals[item.Key] = paidIdx
			l.Cost = LessonCost(paidIdx)
			if item.AlreadyPaid {
				// Урок взят ею самой: номер занят, но опыт за него платим мы.
				l.Note = "уже куплено"
			}
			if item.MinLesson > paidIdx {
				warnings = append(warnings,
					fmt.Sprintf("«%s» требует минимум %d-й урок, а стоит %d-м", item.Title, item.MinLesson, paidIdx))
			}
		}
		total += l.Cost
		lines[SecLessons] = append(lines[SecLessons], l)
	}

	// Позиции, которые план обязан содержать, и условия каркаса.
	for _, it := range p.Shop {
		if it.Locked && !p.Picked(it.Key) {
			warnings = append(warnings,
				fmt.Sprintf("обязательная позиция «%s» не взята%s", it.Title, lockSuffix(&it)))
		}
	}
	for _, req := range p.Requirements {
		if req.Kind == "all_of" {
			for _, k := range req.Keys {
				if !p.Picked(k) {
					warnings = append(warnings, requirementWarning(p, req, k))
				}
			}
			continue
		}
		ok := false
		for _, k := range req.Keys {
			if p.Picked(k) {
				ok = true
				break
			}
		}
		if !ok {
			warnings = append(warnings, requirementWarning(p, req, ""))
		}
	}

	// Отложенные уроки: попадают в план бесплатно, но не нумеруются.
	deferredLimit := 1
	for _, pick := range p.Picks {
		if item := p.Item(pick.Key); item != nil && hasTag(item, "defer_slot") {
			deferredLimit++
		}
	}
	deferredUsed := 0
	for _, d := range p.Deferred {
		item := p.Item(d.Key)
		if item == nil {
			continue
		}
		deferredUsed++
		if item.NoDefer {
			warnings = append(warnings, fmt.Sprintf("«%s» нельзя отложить невыученными уроками", item.Title))
		}
		lines[SecLessons] = append(lines[SecLessons], Line{
			Key: item.Key, Title: displayTitle(item), Deferred: true,
			Note: "невыученный, в следующую жизнь",
		})
	}
	if deferredUsed > deferredLimit {
		warnings = append(warnings, fmt.Sprintf("отложено уроков больше, чем слотов: %d из %d", deferredUsed, deferredLimit))
	}

	// Остальные покупки.
	for _, pick := range p.Picks {
		item := p.Item(pick.Key)
		if item == nil || item.Kind == KindLesson {
			continue
		}
		qty := pick.Qty
		if qty <= 0 {
			qty = 1
		}
		cost := item.Cost * float64(qty)
		total += cost
		sec := sectionOf(item)
		note := item.priceOrNote()
		if item.AlreadyPaid {
			// Позиция взята ею самой: цена обычная, но помечаем это в плане.
			note = "уже куплено"
		}
		lines[sec] = append(lines[sec], Line{
			Key: item.Key, Title: displayTitle(item), Cost: cost, Qty: qty, Note: note,
		})
	}

	// Шкалы (талант, корни, шансы).
	for i := range p.Ladders {
		l := &p.Ladders[i]
		if l.Hidden || l.Value <= l.From {
			continue
		}
		cost := l.ValueCost()
		if cost == 0 {
			continue
		}
		total += cost
		lines[l.Section] = append(lines[l.Section], Line{
			Key: l.Key, Title: ladderTitle(l), Cost: cost, Ladder: true,
		})
	}

	// Итоги по секциям в фиксированном порядке.
	summary := &Summary{
		Balance: p.Balance, Total: total, Remainder: p.Balance - total,
		Warnings: warnings, DeferredLimit: deferredLimit, DeferredUsed: deferredUsed,
	}
	run := p.Balance
	for _, sec := range Sections {
		ls := lines[sec.Key]
		if len(ls) == 0 {
			continue
		}
		ss := SectionSummary{Key: sec.Key, Title: sec.Title, Hint: sec.Hint, Order: sec.Order, Lines: ls}
		for _, l := range ls {
			ss.Sum += l.Cost
		}
		run -= ss.Sum
		ss.Remainder = run
		summary.Sections = append(summary.Sections, ss)
	}
	if summary.Remainder < -1e-9 {
		summary.Warnings = append(summary.Warnings,
			fmt.Sprintf("перерасход: %.2f XP больше, чем есть", -summary.Remainder))
	}

	// «Не взято».
	taken := map[string]bool{}
	for _, pk := range p.Picks {
		taken[pk.Key] = true
	}
	for _, d := range p.Deferred {
		taken[d.Key] = true
	}
	for _, key := range p.NotTaken {
		item := p.Item(key)
		if item == nil {
			continue
		}
		summary.NotTaken = append(summary.NotTaken, Line{Key: item.Key, Title: displayTitle(item), NotTaken: true})
	}
	_ = taken
	sort.SliceStable(summary.NotTaken, func(i, j int) bool {
		return SectionOrder(secOf(summary.NotTaken[i].Key, p)) < SectionOrder(secOf(summary.NotTaken[j].Key, p))
	})
	return summary
}

func secOf(key string, p *Plan) string {
	if item := p.Item(key); item != nil {
		return sectionOf(item)
	}
	return SecOther
}

func sectionOf(item *ShopItem) string {
	if item.Section == "" {
		return SecOther
	}
	return item.Section
}

func displayTitle(item *ShopItem) string {
	if item.Title != "" {
		return item.Title
	}
	return item.Full
}

func hasTag(item *ShopItem, tag string) bool {
	for _, t := range item.Tags {
		if t == tag {
			return true
		}
	}
	return false
}

// lockSuffix добавляет причину блокировки к предупреждению.
func lockSuffix(it *ShopItem) string {
	if it.LockNote != "" {
		return ": " + it.LockNote
	}
	return ""
}

// requirementWarning собирает текст предупреждения по условию каркаса.
func requirementWarning(p *Plan, req Requirement, key string) string {
	title := req.Message
	if title == "" {
		var names []string
		if key != "" {
			names = append(names, itemTitle(p, key))
		} else {
			for _, k := range req.Keys {
				names = append(names, itemTitle(p, k))
			}
		}
		if req.Kind == "all_of" {
			title = "нужно взять: " + strings.Join(names, ", ")
		} else {
			title = "нужно взять одно из: " + strings.Join(names, ", ")
		}
	}
	return "условие каркаса не выполнено — " + title
}

func itemTitle(p *Plan, key string) string {
	if it := p.Item(key); it != nil {
		return displayTitle(it)
	}
	return key
}

func (item *ShopItem) priceOrNote() string {
	if item.Price != "" {
		return item.Price
	}
	return ""
}

func ladderTitle(l *LadderCfg) string {
	val := formatNum(l.Value)
	switch {
	case l.Unit == "d": // кость: пишем dN, а не Nd
		return fmt.Sprintf("%s: d%s", l.Title, val)
	case l.Unit != "":
		return fmt.Sprintf("%s: %s%s", l.Title, val, l.Unit)
	}
	return fmt.Sprintf("%s: %s", l.Title, val)
}

// formatNum печатает число без лишних нулей.
func formatNum(v float64) string {
	if v == float64(int64(v)) {
		return fmt.Sprintf("%d", int64(v))
	}
	s := strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.4f", v), "0"), ".")
	return s
}

// DefaultLadders — базовые шкалы, известные из текста книги.
func DefaultLadders() []LadderCfg {
	return []LadderCfg{
		{Key: "talent.die", Section: SecTalent, Title: "кость таланта", Unit: "d", From: 1, To: 20, Step: 1,
			Cost: LadderCost{Type: "to_value", Rate: 1},
			Note: "цена кости: dN стоит N XP"},
		{Key: "talent.dice_count", Section: SecTalent, Title: "число костей таланта", From: 1, To: 4, Step: 1,
			Cost: LadderCost{Type: "table", Table: []float64{0, 0, 10, 20, 40, 80, 160}},
			Note: "вторая кость — 10 XP, третья — 20 XP, четвёртая — 40 XP"},
		{Key: "talent.flat", Section: SecTalent, Title: "плоский бонус к таланту", From: 0, To: 4, Step: 1,
			Cost: LadderCost{Type: "table", Table: []float64{0, 5, 10, 15, 20, 25, 30}},
			Note: "+1 — 5 XP, дальше 10/15/20 XP"},
		{Key: "roots.max", Section: SecRoots, Title: "максимальная чистота корней", Unit: "%", From: 0, To: 100, Step: 5,
			Cost: LadderCost{Type: "linear", Rate: 5},
			Note: "+5% за 5 XP"},
		{Key: "roots.min", Section: SecRoots, Title: "минимальная чистота корней", Unit: "%", From: 0, To: 100, Step: 1,
			Cost: LadderCost{Type: "to_value", Rate: 1},
			Note: "каждый следующий процент дороже на 1 XP"},
		{Key: "chance.billion", Section: SecChances, Title: "шанс «один на миллиард»", From: 0, To: 3, Step: 1,
			Cost: LadderCost{Type: "table", Table: []float64{0, 1, 10, 200}}},
		{Key: "chance.million", Section: SecChances, Title: "шанс «один на миллион»", From: 0, To: 2, Step: 1,
			Cost: LadderCost{Type: "table", Table: []float64{0, 10, 50}}},
		{Key: "chance.million_home", Section: SecChances, Title: "шанс «один на миллион у вас дома»", Unit: "%", From: 0, To: 10, Step: 1,
			Cost: LadderCost{Type: "per_level", PerLevel: 50},
			Note: "первый процент 50 XP, каждый следующий на 50 XP дороже"},
	}
}
