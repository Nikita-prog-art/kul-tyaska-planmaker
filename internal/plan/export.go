package plan

import (
	"fmt"
	"strings"
)

// Render выводит план в текстовом формате, принятом для планов культяшки:
// имя со смайликом, по одной строке на раздел, суммы и остатки по разделам,
// невыученные уроки, не взятые уроки и итоговый остаток.
//
// В тексте нет переносов внутри строк: длинный раздел остаётся одной строкой,
// а переносит его уже тот, кто читает, — чат или страница экспорта. Так план
// копируется одним куском, и разметка не зависит от ширины окна.
func Render(p *Plan, s *Summary) string {
	var b strings.Builder
	fmt.Fprintf(&b, "[*] %s", strings.TrimSpace(p.Title))
	if strings.TrimSpace(p.Emoji) != "" {
		fmt.Fprintf(&b, " %s", strings.TrimSpace(p.Emoji))
	}
	b.WriteString("\n\n")

	// Проваленные проверки видно сразу: план читает Ли Лун, а не только игрок.
	if len(s.Warnings) > 0 {
		fmt.Fprintf(&b, "⚠ Проверки не выполняются: %d\n\n", len(s.Warnings))
	}

	for _, sec := range s.Sections {
		var parts []string
		for _, l := range sec.Lines {
			if l.Deferred {
				continue // отложенные уроки перечислены в «Невыученных»
			}
			parts = append(parts, lineText(p, l))
		}
		if len(parts) == 0 {
			continue
		}
		fmt.Fprintf(&b, "%s: %s\n", sec.Title, strings.Join(parts, ", "))
		fmt.Fprintf(&b, "  Сумма: %s, ост. %s\n\n", money(p, sec.Sum), money(p, sec.Remainder))
	}

	// Накопители и фолианты — по строке, как и остальные разделы.
	// Перечисляем только то, что берём: список «не активирую» и «оставляю
	// в библиотеке» ничего не добавляет, а место в сообщении занимает.
	var act []string
	for i := range p.Accumulators {
		if !p.Accumulators[i].Activate {
			continue
		}
		act = append(act, accumulatorLine(p, &p.Accumulators[i]))
	}
	if len(act) > 0 {
		fmt.Fprintf(&b, "Активация накопителей: %s\n\n", strings.Join(act, ", "))
	}

	var take []string
	for i := range p.Folios {
		f := &p.Folios[i]
		if !f.Take {
			continue
		}
		line := f.ShortName()
		if f.Note != "" {
			line += " (" + f.Note + ")"
		}
		take = append(take, line)
	}
	if len(take) > 0 {
		fmt.Fprintf(&b, "Беру с собой: %s\n\n", strings.Join(take, ", "))
	}

	if len(s.DeferredUsedKeys(p)) > 0 {
		fmt.Fprintf(&b, "Невыученные: %s\n\n", strings.Join(s.DeferredUsedKeys(p), ", "))
	} else {
		b.WriteString("Невыученные: —\n\n")
	}

	if len(s.NotTaken) > 0 {
		var names []string
		for _, l := range s.NotTaken {
			names = append(names, lineName(l))
		}
		fmt.Fprintf(&b, "Не взято: %s\n\n", strings.Join(names, ", "))
	}

	if len(s.Warnings) > 0 {
		fmt.Fprintf(&b, "Проверки: %s\n\n", strings.Join(s.Warnings, "; "))
	}

	fmt.Fprintf(&b, "Итого потрачено: %s\n", money(p, s.Total))
	fmt.Fprintf(&b, "Остаток: %s XP\n", tail(p, s.Remainder))
	return b.String()
}

// lineText печатает одну позицию плана: «1. Мелисандра: 5 (уже куплено)»,
// «водное тело: 0», «хардкор: 20», «корни мин 50%: 481».
func lineText(p *Plan, l Line) string {
	name := lineName(l)
	if l.Qty > 1 {
		name = fmt.Sprintf("%s ×%d", name, l.Qty)
	}
	if l.Number > 0 {
		return fmt.Sprintf("%d. %s: %s%s", l.Number, name, money(p, l.Cost), noteSuffix(l.Note))
	}
	return fmt.Sprintf("%s: %s%s", name, money(p, l.Cost), noteSuffix(l.Note))
}

// lineName — название строки для выгрузки: короткое, если оно есть.
func lineName(l Line) string {
	if l.Short != "" {
		return l.Short
	}
	return l.Title
}

// accumulatorLine печатает накопитель с состоянием на момент перерождения:
// заряды и максимум с учётом уже сделанных покупок. Если про заряды в тексте
// ничего не известно, печатается только название.
func accumulatorLine(p *Plan, a *Accumulator) string {
	if !p.AccumulatorKnown(a) {
		return a.ShortName()
	}
	charges, max, rate := p.AccumulatorValues(a)
	line := fmt.Sprintf("%s (%s/%s", a.ShortName(), num(p, charges), num(p, max))
	if rate != 0 {
		line += fmt.Sprintf(", +%s%% за жизнь", num(p, rate))
	}
	return line + ")"
}

// noteSuffix печатает пометку к строке плана («уже куплено» и т. п.).
func noteSuffix(note string) string {
	if note == "" {
		return ""
	}
	return " (" + note + ")"
}

// DeferredUsedKeys возвращает краткие названия отложенных уроков в порядке слотов.
func (s *Summary) DeferredUsedKeys(p *Plan) []string {
	var out []string
	for _, d := range p.Deferred {
		if item := p.Item(d.Key); item != nil {
			out = append(out, item.ShortName())
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
