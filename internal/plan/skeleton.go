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
	}
	for i := range s.Accumulators {
		a := &s.Accumulators[i]
		if a.Max <= 0 {
			a.Max = 1
		}
		if a.Key == "" {
			a.Key = "acc:" + Slug(a.Name)
		}
	}
	return nil
}

// NewPlan создаёт план на основе каркаса.
func (s *Skeleton) NewPlan(userID, title, emoji string) *Plan {
	if strings.TrimSpace(title) == "" {
		title = s.Name
	}
	return &Plan{
		ID: newID(), UserID: userID, Title: title, Emoji: emoji,
		Balance:  s.Balance,
		Skeleton: s.Name,
		Earned:   s.Earned,
		Subtotal: s.Subtotal,
		Mults:    s.Multipliers,
		Source:   s.Source,
		Notes:    s.Notes,
		Shop:     s.Shop,
		Ladders:  s.Ladders,
		Accumulators: s.Accumulators,
		Folios:   s.Folios,
		Options:  Options{Decimals: true, Numbered: true},
	}
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
