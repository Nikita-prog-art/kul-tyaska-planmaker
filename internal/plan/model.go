// Package plan описывает модель плана перерождения, каталог покупок
// (магазин посмертия) и движок подсчёта стоимости.
package plan

import "time"

// Kind — тип записи магазина.
const (
	KindLesson      = "lesson"      // урок (стоимость растёт с номером)
	KindPatch       = "patch"       // заплатка
	KindItem        = "item"        // обычная покупка фиксированной цены
	KindAccumulator = "accumulator" // накопитель
	KindFolio       = "folio"       // фолиант (метод/техника)
	KindRevenge     = "revenge"     // месть
	KindLadder      = "ladder"      // параметрическая покупка (шкала)
)

// Секции плана (порядок отображения и суммирования).
const (
	SecLessons      = "lessons"
	SecAccumulators = "accumulators"
	SecTalent       = "talent"
	SecRoots        = "roots"
	SecChances      = "chances"
	SecTrainings    = "trainings"
	SecPhysique     = "physique"
	SecBloodline    = "bloodline"
	SecFolio        = "folio"
	SecPatches      = "patches"
	SecOther        = "other"
)

// ShopItem — одна позиция магазина, доступная в конкретном посмертии.
// Магазин хранится внутри плана: он разобран из текста посмертия и его
// можно править руками, поэтому план самодостаточен.
type ShopItem struct {
	Key       string   `json:"key"`
	Section   string   `json:"section"`
	Kind      string   `json:"kind"`
	Title     string   `json:"title"`          // кратко, как в плане
	Full      string   `json:"full,omitempty"` // полное название из текста
	Cost      float64  `json:"cost"`           // база; для растущих уроков 0
	Fixed     bool     `json:"fixed"`          // фиксированная цена урока
	MinLesson int      `json:"min_lesson"`     // «можно взять как минимум n-ным уроком»
	Once      bool     `json:"once"`
	Requires  []string `json:"requires,omitempty"`
	Excludes  []string `json:"excludes,omitempty"`
	NoDefer   bool     `json:"no_defer,omitempty"` // «нельзя применить невыученные уроки»
	Desc      string   `json:"desc,omitempty"`
	Tags      []string `json:"tags,omitempty"`
	Price     string   `json:"price,omitempty"` // цена как в тексте: «5 XP», «трофей, 0 XP»
}

// Pick — выбранная позиция плана. Порядок в срезе = порядок покупки,
// он же определяет нумерацию уроков и их стоимость.
type Pick struct {
	Key   string  `json:"key"`
	Qty   int     `json:"qty,omitempty"`
	Value float64 `json:"value,omitempty"`
	Note  string  `json:"note,omitempty"`
}

// Deferred — урок, отложенный «невыученными уроками» на следующую жизнь.
type Deferred struct {
	Key  string `json:"key"`
	Slot int    `json:"slot"`
}

// Accumulator — состояние накопителя: заряды, максимум, скорость зарядки.
type Accumulator struct {
	Key      string  `json:"key"`
	Name     string  `json:"name"`
	Activate bool    `json:"activate"`
	Charges  float64 `json:"charges"`
	Max      float64 `json:"max"`
	Rate     float64 `json:"rate"` // % за жизнь
	Note     string  `json:"note,omitempty"`
}

// Folio — фолиант в библиотеке.
type Folio struct {
	Key      string `json:"key"`
	Name     string `json:"name"`
	Take     bool   `json:"take"`     // берём с собой в следующую жизнь
	Deferred bool   `json:"deferred"` // отложен невыученными уроками
	Cost     string `json:"cost,omitempty"`
	Note     string `json:"note,omitempty"`
}

// LadderCost описывает цену шага шкалы.
type LadderCost struct {
	Type     string    `json:"type"` // to_value | linear | per_level | table
	Rate     float64   `json:"rate,omitempty"`
	PerLevel float64   `json:"per_level,omitempty"`
	Table    []float64 `json:"table,omitempty"`
	Ceil     bool      `json:"ceil,omitempty"` // округлять цену шага вверх
}

// LadderCfg — шкала покупки (талант, корни, шансы) вместе с текущим значением.
type LadderCfg struct {
	Key     string     `json:"key"`
	Section string     `json:"section"`
	Title   string     `json:"title"`
	Unit    string     `json:"unit,omitempty"`
	From    float64    `json:"from"`
	To      float64    `json:"to"`
	Step    float64    `json:"step"`
	Value   float64    `json:"value"`
	Cost    LadderCost `json:"cost"`
	Note    string     `json:"note,omitempty"`
	Hidden  bool       `json:"hidden,omitempty"` // не показывать в этой жизни
}

// Options — настройки вывода.
type Options struct {
	Decimals bool `json:"decimals"` // показывать дробную часть XP
	Numbered bool `json:"numbered"` // нумеровать уроки
}

// Plan — план перерождения.
type Plan struct {
	ID      string `json:"id"`
	UserID  string `json:"user_id"`
	Title   string `json:"title"`
	Emoji   string `json:"emoji,omitempty"`
	Balance float64 `json:"balance"`

	// Каркас посмертия: скелет магазина, который готовится под конкретную
	// смерть (обычно его составляет Архивариус) и загружается в план.
	Skeleton string   `json:"skeleton,omitempty"` // название каркаса
	Earned   float64  `json:"earned,omitempty"`   // итог посмертия
	Subtotal float64  `json:"subtotal,omitempty"` // подытог
	Mults    []string `json:"multipliers,omitempty"`
	Source   string   `json:"source,omitempty"` // сырой текст посмертия

	Shop []ShopItem `json:"shop"`

	Picks        []Pick        `json:"picks"`
	Deferred     []Deferred    `json:"deferred"`
	NotTaken     []string      `json:"not_taken"`
	Accumulators []Accumulator `json:"accumulators"`
	Folios       []Folio       `json:"folios"`
	Ladders      []LadderCfg   `json:"ladders"`

	Notes   string  `json:"notes,omitempty"`
	Options Options `json:"options"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Item ищет позицию магазина по ключу.
func (p *Plan) Item(key string) *ShopItem {
	for i := range p.Shop {
		if p.Shop[i].Key == key {
			return &p.Shop[i]
		}
	}
	return nil
}

// Picked сообщает, выбрана ли позиция.
func (p *Plan) Picked(key string) bool {
	for _, pk := range p.Picks {
		if pk.Key == key {
			return true
		}
	}
	return false
}

// IsDeferred сообщает, отложена ли позиция.
func (p *Plan) IsDeferred(key string) bool {
	for _, d := range p.Deferred {
		if d.Key == key {
			return true
		}
	}
	return false
}

// Ladder ищет шкалу по ключу.
func (p *Plan) Ladder(key string) *LadderCfg {
	for i := range p.Ladders {
		if p.Ladders[i].Key == key {
			return &p.Ladders[i]
		}
	}
	return nil
}
