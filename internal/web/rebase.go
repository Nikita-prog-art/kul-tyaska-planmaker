package web

import (
	"strings"
	"time"

	"kultyaska/internal/plan"
)

// sameRevision отвечает, на той ли ревизии плана собран черновик.
// Метка приходит строкой из JSON и из формы: сравниваем и как канонический
// RFC3339Nano, и как момент времени, чтобы метки из apply-preset.py
// (python пишет «…+00:00»/микросекунды) тоже совпадали.
func sameRevision(base string, updated time.Time) bool {
	base = strings.TrimSpace(base)
	if base == "" {
		return false
	}
	if base == updated.UTC().Format(time.RFC3339Nano) {
		return true
	}
	t, err := time.Parse(time.RFC3339Nano, base)
	if err != nil {
		return false
	}
	return t.Equal(updated)
}

// applyEditorState применяет состояние редактора к плану, когда черновик
// собран на текущей ревизии. В этом случае клиент вправе менять и магазин
// (осознанная правка каркаса в плане), и шкалы.
func applyEditorState(p *plan.Plan, st *saveState) {
	if t := strings.TrimSpace(st.Title); t != "" {
		p.Title = t
	}
	p.Emoji = strings.TrimSpace(st.Emoji)
	p.Balance = st.Balance
	p.Notes = st.Notes
	p.Options.Decimals = st.Decimals
	p.Options.Numbered = true
	p.Skeleton = st.Skeleton
	p.Earned = st.Earned
	p.Mults = st.Mults
	if st.Source != "" {
		p.Source = st.Source
	}
	if st.Shop != nil {
		p.Shop = st.Shop
	}
	p.Picks = normalizePicks(p, st.Picks)
	p.Deferred = normalizeDeferred(p, st.Deferred)
	p.NotTaken = normalizeKeys(p, st.NotTaken)
	if st.Ladders != nil {
		p.Ladders = st.Ladders
	}
	if st.Accumulators != nil {
		p.Accumulators = st.Accumulators
	}
	if st.Folios != nil {
		p.Folios = st.Folios
	}
	if st.Requirements != nil {
		p.Requirements = st.Requirements
	}
	if st.DeferredSlots > 0 {
		p.DeferredSlots = st.DeferredSlots
	}
}

// rebaseEditorState переносит выбор игрока со старого каркаса на серверный.
// Магазин, цены, каркас, условия и слоты невыученных остаются серверными:
// именно они только что изменились, и клиентская копия их не знает.
// Покупки переносятся по ключу, а если ключ переименовали — по названию,
// как это делает scripts/apply-preset.py. Возвращает названия потерянных
// позиций, чтобы о них можно было сказать вслух.
func rebaseEditorState(p *plan.Plan, st *saveState) []string {
	if t := strings.TrimSpace(st.Title); t != "" {
		p.Title = t
	}
	p.Emoji = strings.TrimSpace(st.Emoji)
	p.Balance = st.Balance
	p.Notes = st.Notes
	p.Options.Decimals = st.Decimals
	p.Options.Numbered = true

	// Старый магазин клиента — не источник истины, но по нему видно,
	// как назывались позиции, которые в новом каркасе переименовали.
	oldShop := make(map[string]plan.ShopItem, len(st.Shop))
	for _, it := range st.Shop {
		if it.Key != "" {
			oldShop[it.Key] = it
		}
	}
	byTitle := make(map[string]string, len(p.Shop))
	for _, it := range p.Shop {
		if it.Title != "" {
			byTitle[strings.ToLower(it.Title)] = it.Key
		}
	}
	remap := func(key string) (string, bool) {
		if key == "" {
			return "", false
		}
		if p.Item(key) != nil {
			return key, true
		}
		if old, ok := oldShop[key]; ok && old.Title != "" {
			if nk, ok := byTitle[strings.ToLower(old.Title)]; ok {
				return nk, true
			}
		}
		return "", false
	}

	lost := make([]string, 0)
	lostKey := func(key string) {
		title := key
		if old, ok := oldShop[key]; ok && old.Title != "" {
			title = old.Title
		}
		for _, seen := range lost {
			if seen == title {
				return
			}
		}
		lost = append(lost, title)
	}

	picks := make([]plan.Pick, 0, len(st.Picks))
	at := make(map[string]int, len(st.Picks))
	for _, pk := range st.Picks {
		nk, ok := remap(pk.Key)
		if !ok {
			lostKey(pk.Key)
			continue
		}
		pk.Key = nk
		if idx, ok := at[nk]; ok {
			picks[idx].Qty += pk.Qty
			continue
		}
		at[nk] = len(picks)
		picks = append(picks, pk)
	}
	for i := range picks {
		if picks[i].Qty < 1 {
			picks[i].Qty = 1
		}
		if it := p.Item(picks[i].Key); it != nil {
			limit := it.Repeat
			if limit < 1 {
				limit = 1
			}
			if picks[i].Qty > limit {
				picks[i].Qty = limit
			}
		}
	}
	p.Picks = picks

	deferred := make([]plan.Deferred, 0, len(st.Deferred))
	seenDeferred := make(map[string]bool, len(st.Deferred))
	for _, d := range st.Deferred {
		nk, ok := remap(d.Key)
		if !ok {
			lostKey(d.Key)
			continue
		}
		if seenDeferred[nk] {
			continue
		}
		seenDeferred[nk] = true
		deferred = append(deferred, plan.Deferred{Key: nk, Slot: len(deferred) + 1})
	}
	p.Deferred = deferred

	notTaken := make([]string, 0, len(st.NotTaken))
	seenNotTaken := make(map[string]bool, len(st.NotTaken))
	for _, key := range st.NotTaken {
		nk, ok := remap(key)
		if !ok || seenNotTaken[nk] {
			continue
		}
		seenNotTaken[nk] = true
		notTaken = append(notTaken, nk)
	}
	p.NotTaken = notTaken

	// Шкалы: границы, шаги и цены — серверные, значение выбирает игрок.
	if st.Ladders != nil {
		oldLadders := make(map[string]plan.LadderCfg, len(st.Ladders))
		for _, l := range st.Ladders {
			oldLadders[l.Key] = l
		}
		for i := range p.Ladders {
			old, ok := oldLadders[p.Ladders[i].Key]
			if !ok {
				continue
			}
			v := old.Value
			if v < p.Ladders[i].From {
				v = p.Ladders[i].From
			}
			if v > p.Ladders[i].To {
				v = p.Ladders[i].To
			}
			p.Ladders[i].Value = v
		}
	}

	// Отметки накопителей и фолиантов — действия игрока; остальное
	// (заряды, максимум, цена) берём из серверного каркаса.
	if st.Accumulators != nil {
		for i := range p.Accumulators {
			if old, ok := findAccumulator(st.Accumulators, p.Accumulators[i].Key, p.Accumulators[i].Name); ok {
				p.Accumulators[i].Activate = old.Activate
			}
		}
	}
	if st.Folios != nil {
		for i := range p.Folios {
			if old, ok := findFolio(st.Folios, p.Folios[i].Key, p.Folios[i].Name); ok {
				p.Folios[i].Take = old.Take
				p.Folios[i].Deferred = old.Deferred
			}
		}
	}
	return lost
}

func findAccumulator(list []plan.Accumulator, key, name string) (plan.Accumulator, bool) {
	for _, a := range list {
		if (key != "" && a.Key == key) || (name != "" && a.Name == name) {
			return a, true
		}
	}
	return plan.Accumulator{}, false
}

func findFolio(list []plan.Folio, key, name string) (plan.Folio, bool) {
	for _, f := range list {
		if (key != "" && f.Key == key) || (name != "" && f.Name == name) {
			return f, true
		}
	}
	return plan.Folio{}, false
}
