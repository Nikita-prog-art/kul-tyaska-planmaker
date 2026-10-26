// Package plan описывает модель плана перерождения, каталог покупок
// (магазин посмертия) и движок подсчёта стоимости.
package plan

import (
	"strings"
	"time"
)

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
	SecLessons = "lessons"
	// SecFixed — уроки с фиксированной ценой: трофеи и покупки, которые
	// не занимают номер и не поднимают цену следующих. В плане это отдельный
	// раздел, чтобы нумерация уроков не перемешивалась с ними.
	SecFixed        = "fixed"
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
	// Repeat — сколько раз позицию можно взять в одном плане (0 или 1 — один раз):
	// «взять 1 фолиант (максимум 3)», «лестница в небо — можно купить дважды».
	Repeat int      `json:"repeat,omitempty"`
	Desc   string   `json:"desc,omitempty"`
	Tags   []string `json:"tags,omitempty"`
	Price  string   `json:"price,omitempty"` // цена как в тексте: «5 XP», «трофей, 0 XP»

	// Locked — позиция, которую план не может не содержать: Ли Лун уже взяла её
	// сама или не станет рассматривать план без неё. В конструкторе такие
	// позиции всегда выбраны и снять их нельзя.
	Locked   bool   `json:"locked,omitempty"`
	LockNote string `json:"lock_note,omitempty"`
	// AlreadyPaid — позиция уже взята ею самой до плана: в плане печатается
	// с пометкой «уже куплено», опыт за неё списывается как обычно, а урок
	// занимает свой номер, так что нумерация продолжается со следующего.
	AlreadyPaid bool `json:"already_paid,omitempty"`
	// RewardAfter — награда за порядок: позицию надо взять ПОСЛЕ другой
	// позиции, иначе награда теряется («последний бой» после «пинг систем»).
	RewardAfter *RewardAfter `json:"reward_after,omitempty"`
	// Effects — что покупка делает с накопителями: «заряд великого успеха»
	// добавляет заряд, «максимум зарядов» поднимает максимум, «скорость
	// зарядки» задаёт новый процент за жизнь.
	Effects []ShopEffect `json:"effects,omitempty"`
}

// ShopEffect — влияние покупки на накопитель.
type ShopEffect struct {
	Accumulator string  `json:"accumulator"`       // ключ накопителя
	Charges     float64 `json:"charges,omitempty"` // +N зарядов сразу
	Max         float64 `json:"max,omitempty"`     // +N к максимуму
	Rate        float64 `json:"rate,omitempty"`    // новый процент за жизнь
}

// RewardAfter — условие «возьми это после того»: если позиция стоит в плане
// раньше указанной, награда не достаётся.
type RewardAfter struct {
	Key   string `json:"key"`
	Title string `json:"title,omitempty"` // название награды, по умолчанию «квестовая награда»
	Note  string `json:"note,omitempty"`  // что именно даёт награда
	Lose  string `json:"lose,omitempty"`  // что теряется, если взять раньше
}

// TitleOr возвращает название награды.
func (r *RewardAfter) TitleOr() string {
	if r == nil || r.Title == "" {
		return "квестовая награда"
	}
	return r.Title
}

// Requirement — условие, которому должен удовлетворять план
// (например, «взять контроль пола ИЛИ фолиант после определения пола»).
type Requirement struct {
	Kind    string   `json:"kind"` // any_of | all_of
	Keys    []string `json:"keys"`
	Message string   `json:"message"`
	// Soft — условие «на будущие жизни»: его невыполнение ничего не ломает
	// (например, повторная встреча с Джоном Сноу возможна только через жизнь),
	// поэтому оно не попадает в «Проверки», а печатается отдельной памяткой.
	Soft bool `json:"soft,omitempty"`
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
	// Requires — покупки, без которых накопитель не активировать:
	// «хардкор» и «равновесие зла и добра» сначала надо купить в магазине.
	Requires []string `json:"requires,omitempty"`
}

// Folio — фолиант в библиотеке.
type Folio struct {
	Key      string `json:"key"`
	Name     string `json:"name"`
	Take     bool   `json:"take"`     // берём с собой в следующую жизнь
	Deferred bool   `json:"deferred"` // отложен невыученными уроками
	Cost     string `json:"cost,omitempty"`
	Note     string `json:"note,omitempty"`
	// Requires — что нужно сделать, чтобы фолиант можно было взять с собой:
	// сначала урок метода, потом выкуп (сам выбор бесплатный).
	Requires []string `json:"requires,omitempty"`
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
	ID      string  `json:"id"`
	UserID  string  `json:"user_id"`
	Title   string  `json:"title"`
	Emoji   string  `json:"emoji,omitempty"`
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
	Requirements []Requirement `json:"requirements,omitempty"`
	// DeferredSlots — сколько слотов «невыученных уроков» открыто к этой смерти:
	// число приходит из каркаса, урок с тегом defer_slot добавляет ещё один.
	DeferredSlots int `json:"deferred_slots,omitempty"`

	Notes   string  `json:"notes,omitempty"`
	Options Options `json:"options"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Normalize приводит план к предсказуемому виду: все списки — не nil
// (иначе в JSON уезжает null, а конструктор в браузере ждёт массивы),
// шкалы и настройки заполняются значениями по умолчанию.
func (p *Plan) Normalize() {
	if p.Title == "" {
		p.Title = "Новый план"
	}
	if p.Shop == nil {
		p.Shop = []ShopItem{}
	}
	if p.Picks == nil {
		p.Picks = []Pick{}
	}
	if p.Deferred == nil {
		p.Deferred = []Deferred{}
	}
	if p.NotTaken == nil {
		p.NotTaken = []string{}
	}
	if p.Ladders == nil {
		p.Ladders = DefaultLadders()
	}
	if p.Accumulators == nil {
		p.Accumulators = []Accumulator{}
	}
	if p.Folios == nil {
		p.Folios = []Folio{}
	}
	if p.Mults == nil {
		p.Mults = []string{}
	}
	if p.DeferredSlots < 1 {
		p.DeferredSlots = 1
	}
	for i := range p.Shop {
		it := &p.Shop[i]
		if it.Kind == "" {
			it.Kind = KindItem
		}
		if it.Section == "" {
			it.Section = SecOther
		}
		if it.MinLesson < 1 {
			it.MinLesson = 1
		}
		if it.Key == "" {
			it.Key = it.Kind + ":" + Slug(it.Title)
		}
		if it.AlreadyPaid {
			it.Locked = true
		}
	}
	// Награда за порядок бессмысленна, если опорной позиции нет в магазине.
	for i := range p.Shop {
		it := &p.Shop[i]
		if it.RewardAfter == nil {
			continue
		}
		found := false
		for _, other := range p.Shop {
			if other.Key == it.RewardAfter.Key {
				found = true
				break
			}
		}
		if !found {
			it.RewardAfter = nil
		}
	}
	for i := range p.Ladders {
		l := &p.Ladders[i]
		if l.Step <= 0 {
			l.Step = 1
		}
		if l.Section == "" {
			l.Section = SecOther
		}
		if l.Title == "" {
			l.Title = l.Key
		}
	}
	p.dropLadderDuplicates()
	// Количество покупок не выходит за разрешённый повтор.
	for i := range p.Picks {
		pk := &p.Picks[i]
		if pk.Qty < 1 {
			pk.Qty = 1
		}
		if it := p.Item(pk.Key); it != nil && it.Repeat > 1 && pk.Qty > it.Repeat {
			pk.Qty = it.Repeat
		}
	}
	// Ссылки в requires — только на существующие покупки: битый ключ (каркас
	// правили руками) запер бы накопитель или фолиант навсегда.
	for i := range p.Shop {
		p.Shop[i].Requires = p.liveKeys(p.Shop[i].Requires)
	}
	for i := range p.Accumulators {
		p.Accumulators[i].Requires = p.liveKeys(p.Accumulators[i].Requires)
	}
	for i := range p.Folios {
		p.Folios[i].Requires = p.liveKeys(p.Folios[i].Requires)
	}
	p.enforceLocked()
	// Отметки «активирую» и «беру» план обязан заслужить: накопитель —
	// зарядом, фолиант — уроком метода и выкупом.
	p.pruneAttachments()
}

// dropLadderDuplicates убирает покупки, которые повторяют шкалу: каркас одно
// время привозил «шанс «один на миллиард»» дважды — ступенью в «Шансах» и
// такой же галочкой в магазине, — так что одну и ту же позицию можно было
// взять за 400 XP. Шкала помнит, сколько ступеней уже куплено, поэтому
// остаётся она; покупка и её выбор из плана уходят.
func (p *Plan) dropLadderDuplicates() {
	if len(p.Ladders) == 0 || len(p.Shop) == 0 {
		return
	}
	dupes := map[string]bool{}
	for _, l := range p.Ladders {
		if l.Hidden || l.Section == "" || l.Title == "" {
			continue
		}
		dupes[dedupeKey(l.Section, ladderBaseTitle(l.Title))] = true
	}
	if len(dupes) == 0 {
		return
	}
	removed := map[string]bool{}
	keep := make([]ShopItem, 0, len(p.Shop))
	for _, it := range p.Shop {
		if it.Kind != KindLadder && dupes[dedupeKey(it.Section, it.Title)] {
			removed[it.Key] = true
			continue
		}
		keep = append(keep, it)
	}
	if len(removed) == 0 {
		return
	}
	p.Shop = keep
	picks := make([]Pick, 0, len(p.Picks))
	for _, pk := range p.Picks {
		if !removed[pk.Key] {
			picks = append(picks, pk)
		}
	}
	p.Picks = picks
	left := make([]string, 0, len(p.NotTaken))
	for _, key := range p.NotTaken {
		if !removed[key] {
			left = append(left, key)
		}
	}
	p.NotTaken = left
}

// ladderBaseTitle отбрасывает у названия шкалы слово «ступень», чтобы
// сравнить её с покупкой: «шанс «один на миллиард», ступень» и «шанс «один
// на миллиард»» — это одна и та же позиция.
func ladderBaseTitle(title string) string {
	t := strings.TrimSpace(title)
	for _, cut := range []string{", ступень", " (ступень)"} {
		if strings.HasSuffix(t, cut) {
			return strings.TrimSpace(strings.TrimSuffix(t, cut))
		}
	}
	return t
}

// dedupeKey — ключ сравнения позиций: секция плюс название без регистра.
func dedupeKey(section, title string) string {
	return strings.ToLower(strings.TrimSpace(section) + "\x00" + strings.TrimSpace(title))
}

// enforceLocked доставляет в план позиции, которые он обязан содержать
// (Ли Лун уже взяла их сама или не станет смотреть план без них).
// Уже оплаченные встают в начало очереди — в порядке магазина, — чтобы
// нумерация уроков совпадала с текстом; остальные обязательные позиции
// добавляются в конец: их место игрок выбирает сам.
func (p *Plan) enforceLocked() {
	var front, back []Pick
	have := map[string]bool{}
	for _, pk := range p.Picks {
		have[pk.Key] = true
	}
	for _, it := range p.Shop {
		if !it.Locked || have[it.Key] {
			continue
		}
		if it.AlreadyPaid {
			front = append(front, Pick{Key: it.Key, Qty: 1})
		} else {
			back = append(back, Pick{Key: it.Key, Qty: 1})
		}
		have[it.Key] = true
	}
	if len(front) > 0 || len(back) > 0 {
		picks := make([]Pick, 0, len(front)+len(p.Picks)+len(back))
		picks = append(picks, front...)
		picks = append(picks, p.Picks...)
		picks = append(picks, back...)
		p.Picks = picks
	}
	// Обязательную позицию нельзя отложить невыученными уроками.
	if len(p.Deferred) > 0 {
		kept := p.Deferred[:0]
		for _, d := range p.Deferred {
			if it := p.Item(d.Key); it != nil && it.Locked {
				continue
			}
			kept = append(kept, d)
		}
		p.Deferred = kept
	}
	if len(p.NotTaken) > 0 {
		kept := p.NotTaken[:0]
		for _, k := range p.NotTaken {
			if it := p.Item(k); it != nil && it.Locked {
				continue
			}
			kept = append(kept, k)
		}
		p.NotTaken = kept
	}
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

// IsLocked сообщает, обязательна ли позиция: план не может её не содержать.
func (p *Plan) IsLocked(key string) bool {
	if it := p.Item(key); it != nil {
		return it.Locked
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
