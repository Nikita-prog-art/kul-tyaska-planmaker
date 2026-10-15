package plan

import "math"

// LessonBase — стоимость первого урока; каждый следующий дороже на LessonStep.
const (
	LessonBase = 5.0
	LessonStep = 5.0
)

// LessonCost возвращает цену урока с указанным номером (нумерация с единицы).
func LessonCost(n int) float64 {
	if n <= 0 {
		return 0
	}
	return LessonBase + LessonStep*float64(n-1)
}

// StepCost возвращает цену перехода шкалы на уровень level.
func (c LadderCost) StepCost(level float64) float64 {
	v := c.rawStepCost(level)
	if c.Ceil && v != 0 {
		v = math.Ceil(v)
	}
	return v
}

func (c LadderCost) rawStepCost(level float64) float64 {
	switch c.Type {
	case "to_value":
		rate := c.Rate
		if rate == 0 {
			rate = 1
		}
		return level * rate
	case "linear":
		return c.Rate
	case "per_level":
		return level * c.PerLevel
	case "table":
		idx := int(math.Round(level))
		if idx >= 0 && idx < len(c.Table) {
			return c.Table[idx]
		}
		return 0
	case "none", "":
		return 0
	}
	return 0
}

// LadderCostUntil суммирует стоимость шкалы от From до target.
func (c LadderCost) LadderCostUntil(from, to, step, target float64) float64 {
	if step <= 0 {
		step = 1
	}
	if target < from {
		target = from
	}
	if to > 0 && target > to {
		target = to
	}
	total := 0.0
	for v := from + step; v <= target+1e-9; v += step {
		total += c.StepCost(v)
	}
	return total
}

// LadderValueCost — стоимость текущего значения шкалы.
func (l LadderCfg) ValueCost() float64 {
	return l.Cost.LadderCostUntil(l.From, l.To, l.Step, l.Value)
}

// Section — описание секции плана.
type Section struct {
	Key   string `json:"key"`
	Title string `json:"title"`
	Hint  string `json:"hint,omitempty"`
	Order int    `json:"order"`
}

// Sections — упорядоченный список секций плана.
var Sections = []Section{
	{Key: SecLessons, Title: "Уроки", Hint: "первый урок 5 XP, каждый следующий на 5 XP дороже", Order: 1},
	{Key: SecPatches, Title: "Заплатки", Hint: "бесплатные и платные правки правил", Order: 2},
	{Key: SecTalent, Title: "Талант", Hint: "кости таланта и плоский бонус", Order: 3},
	{Key: SecRoots, Title: "Чистота корней", Hint: "максимум и минимум", Order: 4},
	{Key: SecChances, Title: "Шансы", Hint: "«один на миллион» и «один на миллиард»", Order: 5},
	{Key: SecAccumulators, Title: "Накопители", Hint: "заряжаются за прожитые жизни", Order: 6},
	{Key: SecPhysique, Title: "Телосложения", Hint: "активное и неактивные", Order: 7},
	{Key: SecBloodline, Title: "Родословные", Hint: "активная и неактивные", Order: 8},
	{Key: SecTrainings, Title: "Тренировки и навыки", Hint: "постоянные бонусы", Order: 9},
	{Key: SecFolio, Title: "Фолианты", Hint: "методы и техники в библиотеке", Order: 10},
	{Key: SecOther, Title: "Прочее", Hint: "всё остальное", Order: 11},
}

// SectionTitle возвращает заголовок секции по ключу.
func SectionTitle(key string) string {
	for _, s := range Sections {
		if s.Key == key {
			return s.Title
		}
	}
	return "Прочее"
}

// SectionOrder возвращает порядковый номер секции.
func SectionOrder(key string) int {
	for _, s := range Sections {
		if s.Key == key {
			return s.Order
		}
	}
	return 99
}
