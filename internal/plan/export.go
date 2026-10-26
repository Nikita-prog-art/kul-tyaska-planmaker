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

	// Проваленные проверки видно сразу: план читает Ли Лун, а не только игрок.
	if len(s.Warnings) > 0 {
		fmt.Fprintf(&b, "⚠ Проверки не выполняются: %d\n\n", len(s.Warnings))
	}

	for _, sec := range s.Sections {
		// Уроки идут одной строкой: «1. поцелуй Мелисандры: 5, 2. …».
		if sec.Key == SecLessons {
			writeLessonsLine(&b, p, sec)
			continue
		}
		fmt.Fprintf(&b, "%s:\n", sec.Title)
		for _, l := range sec.Lines {
			title := l.Title
			if l.Qty > 1 {
				title = fmt.Sprintf("%s ×%d", title, l.Qty)
			}
			switch {
			case l.Deferred:
				note := l.Note
				if note == "" {
					note = "невыученный, в следующую жизнь"
				}
				fmt.Fprintf(&b, "  - %s (%s)\n", title, note)
			case l.Number > 0:
				fmt.Fprintf(&b, "  %d. %s: %s%s\n", l.Number, title, money(p, l.Cost), noteSuffix(l.Note))
			default:
				fmt.Fprintf(&b, "  - %s: %s%s\n", title, money(p, l.Cost), noteSuffix(l.Note))
			}
		}
		fmt.Fprintf(&b, "  Сумма: %s, ост. %s\n\n", money(p, sec.Sum), money(p, sec.Remainder))
	}

	// Накопители и фолианты — отдельными строками, как требует формат.
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
		fmt.Fprintf(&b, "Активация накопителей:\n  - активирую: %s\n\n", strings.Join(act, "; "))
	}

	var take []string
	for _, f := range p.Folios {
		if !f.Take {
			continue
		}
		line := f.Name
		if f.Note != "" {
			line += " (" + f.Note + ")"
		}
		take = append(take, line)
	}
	if len(take) > 0 {
		fmt.Fprintf(&b, "Фолианты:\n  - беру с собой: %s\n\n", strings.Join(take, "; "))
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

	// Условия, которые касаются будущих жизней: памятка, а не проверка плана.
	if len(s.Future) > 0 {
		b.WriteString("На будущие жизни:\n")
		for _, f := range s.Future {
			mark := "— пока не выполнено"
			if f.Met {
				mark = "— выполнено"
			}
			fmt.Fprintf(&b, "  - %s %s\n", strings.TrimSpace(f.Text), mark)
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

// writeLessonsLine печатает уроки одной строкой, как требует формат плана:
// «Уроки: 1. поцелуй Мелисандры: 5 (уже куплено), 2. да сдохни же ты наконец!: 10».
// Отложенные уроки сюда не попадают: они бесплатны и перечислены в «Невыученных».
func writeLessonsLine(b *strings.Builder, p *Plan, sec SectionSummary) {
	var parts []string
	for _, l := range sec.Lines {
		if l.Number == 0 {
			continue
		}
		parts = append(parts, fmt.Sprintf("%d. %s: %s%s", l.Number, l.Title, money(p, l.Cost), noteSuffix(l.Note)))
	}
	if len(parts) == 0 {
		return
	}
	fmt.Fprintf(b, "%s: %s\n", sec.Title, strings.Join(parts, ", "))
	fmt.Fprintf(b, "  Сумма: %s, ост. %s\n\n", money(p, sec.Sum), money(p, sec.Remainder))
}

// accumulatorLine печатает накопитель с состоянием на момент перерождения:
// заряды и максимум с учётом уже сделанных покупок. Если про заряды в тексте
// ничего не известно, печатается только название.
func accumulatorLine(p *Plan, a *Accumulator) string {
	if !p.AccumulatorKnown(a) {
		return a.Name
	}
	charges, max, rate := p.AccumulatorValues(a)
	line := fmt.Sprintf("%s (%s/%s", a.Name, num(p, charges), num(p, max))
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
