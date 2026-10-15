package plan

import (
	"fmt"
	"strings"
)

// Render выводит план в текстовом формате, принятом для планов культяшки:
// имя со смайликом, краткие строки, суммы и остатки по секциям,
// невыученные уроки, не взятые уроки и итоговый остаток.
func Render(p *Plan, s *Summary) string {
	var b strings.Builder
	fmt.Fprintf(&b, "[*] %s", strings.TrimSpace(p.Title))
	if strings.TrimSpace(p.Emoji) != "" {
		fmt.Fprintf(&b, " %s", strings.TrimSpace(p.Emoji))
	}
	b.WriteString("\n\n")

	for _, sec := range s.Sections {
		fmt.Fprintf(&b, "%s:\n", sec.Title)
		for _, l := range sec.Lines {
			switch {
			case l.Deferred:
				fmt.Fprintf(&b, "  - %s (невыученный, в следующую жизнь)\n", l.Title)
			case l.Number > 0:
				fmt.Fprintf(&b, "  %d. %s: %s\n", l.Number, l.Title, money(p, l.Cost))
			default:
				fmt.Fprintf(&b, "  - %s: %s\n", l.Title, money(p, l.Cost))
			}
		}
		fmt.Fprintf(&b, "  Сумма: %s, ост. %s\n\n", money(p, sec.Sum), money(p, sec.Remainder))
	}

	// Накопители и фолианты — отдельными строками, как требует формат.
	var act, skip []string
	for _, a := range p.Accumulators {
		line := fmt.Sprintf("%s (%s/%s, +%s%% за жизнь)", a.Name, num(p, a.Charges), num(p, a.Max), num(p, a.Rate))
		if a.Activate {
			act = append(act, line)
		} else {
			skip = append(skip, line)
		}
	}
	if len(act) > 0 || len(skip) > 0 {
		b.WriteString("Накопители:\n")
		if len(act) > 0 {
			fmt.Fprintf(&b, "  - активирую: %s\n", strings.Join(act, "; "))
		}
		if len(skip) > 0 {
			fmt.Fprintf(&b, "  - не активирую: %s\n", strings.Join(skip, "; "))
		}
		b.WriteString("\n")
	}

	var take, keep []string
	for _, f := range p.Folios {
		line := f.Name
		if f.Note != "" {
			line += " (" + f.Note + ")"
		}
		if f.Take {
			take = append(take, line)
		} else {
			keep = append(keep, line)
		}
	}
	if len(take) > 0 || len(keep) > 0 {
		b.WriteString("Фолианты:\n")
		if len(take) > 0 {
			fmt.Fprintf(&b, "  - беру с собой: %s\n", strings.Join(take, "; "))
		}
		if len(keep) > 0 {
			fmt.Fprintf(&b, "  - оставляю в библиотеке: %s\n", strings.Join(keep, "; "))
		}
		b.WriteString("\n")
	}

	if len(s.DeferredUsedKeys(p)) > 0 {
		fmt.Fprintf(&b, "Невыученные: %s\n\n", strings.Join(s.DeferredUsedKeys(p), ", "))
	} else {
		b.WriteString("Невыученные: —\n\n")
	}

	if len(s.NotTaken) > 0 {
		var names []string
		for _, l := range s.NotTaken {
			names = append(names, l.Title)
		}
		fmt.Fprintf(&b, "Не взято: %s\n\n", strings.Join(names, ", "))
	}

	if len(s.Warnings) > 0 {
		b.WriteString("Проверки:\n")
		for _, w := range s.Warnings {
			fmt.Fprintf(&b, "  ! %s\n", w)
		}
		b.WriteString("\n")
	}

	fmt.Fprintf(&b, "Итого потрачено: %s\n", money(p, s.Total))
	fmt.Fprintf(&b, "Остаток: %s XP\n", tail(p, s.Remainder))
	if p.Notes != "" {
		fmt.Fprintf(&b, "\n%s\n", strings.TrimSpace(p.Notes))
	}
	return b.String()
}

// DeferredUsedKeys возвращает краткие названия отложенных уроков в порядке слотов.
func (s *Summary) DeferredUsedKeys(p *Plan) []string {
	var out []string
	for _, d := range p.Deferred {
		if item := p.Item(d.Key); item != nil {
			out = append(out, displayTitle(item))
		}
	}
	return out
}

// num форматирует число с отключёнными/включёнными дробями.
func num(p *Plan, v float64) string {
	if !p.Options.Decimals {
		return fmt.Sprintf("%d", int64(v+0.5*sign(v)))
	}
	return comma(v, 5, true)
}

// money форматирует стоимость (без суффикса XP).
func money(p *Plan, v float64) string {
	if !p.Options.Decimals {
		return fmt.Sprintf("%d", int64(v+0.5*sign(v)))
	}
	return comma(v, 5, true)
}

// tail форматирует итоговый остаток: дроби оставляем всегда, если включены.
func tail(p *Plan, v float64) string {
	if !p.Options.Decimals {
		return fmt.Sprintf("%d", int64(v+0.5*sign(v)))
	}
	return comma(v, 5, true)
}

func sign(v float64) float64 {
	if v < 0 {
		return -1
	}
	return 1
}

// comma печатает число с запятой в качестве десятичного разделителя.
func comma(v float64, prec int, trim bool) string {
	s := fmt.Sprintf("%.*f", prec, v)
	if trim && strings.Contains(s, ".") {
		s = strings.TrimRight(s, "0")
		s = strings.TrimRight(s, ".")
	}
	return strings.Replace(s, ".", ",", 1)
}
