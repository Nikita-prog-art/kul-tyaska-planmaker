package plan

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Skeleton — «каркас» посмертия: скелет магазина под конкретную смерть.
// Каркасы готовятся вручную (или Архивариусом) в формате JSON и
// загружаются в план; приложение само ничего из текста не вытаскивает.
type Skeleton struct {
	Name         string        `json:"name"`
	Balance      float64       `json:"balance"`
	Earned       float64       `json:"earned,omitempty"`
	Subtotal     float64       `json:"subtotal,omitempty"`
	Multipliers  []string      `json:"multipliers,omitempty"`
	Source       string        `json:"source,omitempty"`
	Notes        string        `json:"notes,omitempty"`
	Shop         []ShopItem    `json:"shop"`
	Ladders      []LadderCfg   `json:"ladders,omitempty"`
	Accumulators []Accumulator `json:"accumulators,omitempty"`
	Folios       []Folio       `json:"folios,omitempty"`
	// Requirements — условия, которым обязан удовлетворять план
	// (например, «контроль пола» ИЛИ «взять фолиант после определения пола»).
	Requirements []Requirement `json:"requirements,omitempty"`
}

// ParseSkeleton читает каркас из JSON.
func ParseSkeleton(raw []byte) (*Skeleton, error) {
	var s Skeleton
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&s); err != nil {
		// Второй шанс: неизвестные поля просто игнорируем.
		if err2 := json.Unmarshal(raw, &s); err2 != nil {
			return nil, fmt.Errorf("не удалось разобрать каркас: %w", err)
		}
	}
	if err := s.Normalize(); err != nil {
		return nil, err
	}
	return &s, nil
}

// Normalize заполняет пропуски и проверяет каркас.
func (s *Skeleton) Normalize() error {
	if strings.TrimSpace(s.Name) == "" {
		s.Name = "Посмертие без названия"
	}
	if len(s.Shop) == 0 {
		return fmt.Errorf("в каркасе нет ни одной позиции магазина (shop)")
	}
	if len(s.Ladders) == 0 {
		s.Ladders = DefaultLadders()
	}
	for i := range s.Ladders {
		l := &s.Ladders[i]
		if l.Step <= 0 {
			l.Step = 1
		}
		if l.Section == "" {
			l.Section = SecOther
		}
		if l.Title == "" {
			l.Title = l.Key
		}
		if l.Key == "" {
			l.Key = "ladder:" + Slug(l.Title)
		}
	}
	seen := map[string]bool{}
	for i := range s.Shop {
		it := &s.Shop[i]
		it.Title = strings.TrimSpace(it.Title)
		if it.Title == "" {
			it.Title = strings.TrimSpace(it.Full)
		}
		if it.Title == "" {
			return fmt.Errorf("позиция магазина №%d без названия", i+1)
		}
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
		for seen[it.Key] {
			it.Key += "_"
		}
		seen[it.Key] = true
		// Уже оплаченную позицию план обязан содержать.
		if it.AlreadyPaid {
			it.Locked = true
		}
		if it.Locked && it.LockNote == "" {
			it.LockNote = "обязательная позиция каркаса"
		}
	}
	// Награды за порядок: ссылаться можно только на существующую позицию.
	for i := range s.Shop {
		it := &s.Shop[i]
		if it.RewardAfter == nil {
			continue
		}
		if !seen[it.RewardAfter.Key] {
			it.RewardAfter = nil
			continue
		}
		it.RewardAfter.Title = strings.TrimSpace(it.RewardAfter.Title)
	}
	// Условия каркаса: ключи должны существовать в магазине.
	var reqs []Requirement
	for _, req := range s.Requirements {
		if req.Kind == "" {
			req.Kind = "any_of"
		}
		var keys []string
		for _, k := range req.Keys {
			for _, it := range s.Shop {
				if it.Key == k {
					keys = append(keys, k)
					break
				}
			}
		}
		if len(keys) == 0 {
			continue
		}
		req.Keys = keys
		reqs = append(reqs, req)
	}
	s.Requirements = reqs
	for i := range s.Accumulators {
		a := &s.Accumulators[i]
		if a.Max <= 0 {
			a.Max = 1
		}
		if a.Key == "" {
			a.Key = "acc:" + Slug(a.Name)
		}
	}
	// Требования ссылаются на позиции магазина: битые ключи выбрасываем,
	// иначе накопитель или фолиант навсегда останутся недоступными.
	for i := range s.Shop {
		s.Shop[i].Requires = existKeys(seen, s.Shop[i].Requires)
	}
	for i := range s.Accumulators {
		s.Accumulators[i].Requires = existKeys(seen, s.Accumulators[i].Requires)
	}
	// Эффекты покупок ссылаются на накопители: чужие ключи тоже не нужны.
	accSeen := map[string]bool{}
	for i := range s.Accumulators {
		accSeen[s.Accumulators[i].Key] = true
	}
	for i := range s.Shop {
		it := &s.Shop[i]
		var effects []ShopEffect
		for _, e := range it.Effects {
			if e.Accumulator == "" || !accSeen[e.Accumulator] {
				continue
			}
			effects = append(effects, e)
		}
		it.Effects = effects
	}
	for i := range s.Folios {
		f := &s.Folios[i]
		if f.Key == "" {
			f.Key = "folio:" + Slug(f.Name)
		}
		f.Requires = existKeys(seen, f.Requires)
	}
	return nil
}

// existKeys оставляет только ключи, которые есть в магазине.
func existKeys(seen map[string]bool, keys []string) []string {
	var out []string
	for _, k := range keys {
		if seen[k] {
			out = append(out, k)
		}
	}
	return out
}

// NewPlan создаёт план на основе каркаса.
func (s *Skeleton) NewPlan(userID, title, emoji string) *Plan {
	if strings.TrimSpace(title) == "" {
		title = s.Name
	}
	p := &Plan{
		ID: newID(), UserID: userID, Title: title, Emoji: emoji,
		Balance:      s.Balance,
		Skeleton:     s.Name,
		Earned:       s.Earned,
		Subtotal:     s.Subtotal,
		Mults:        s.Multipliers,
		Source:       s.Source,
		Notes:        s.Notes,
		Shop:         s.Shop,
		Ladders:      s.Ladders,
		Accumulators: s.Accumulators,
		Folios:       s.Folios,
		Requirements: s.Requirements,
		Options:      Options{Decimals: true, Numbered: true},
	}
	p.Normalize()
	return p
}

// EmptyPlan создаёт план без каркаса: только базовые шкалы.
func EmptyPlan(userID, title, emoji string, balance float64) *Plan {
	return &Plan{
		ID: newID(), UserID: userID, Title: title, Emoji: emoji,
		Balance: balance, Ladders: DefaultLadders(),
		Options: Options{Decimals: true, Numbered: true},
	}
}

func newID() string {
	return genID()
}
