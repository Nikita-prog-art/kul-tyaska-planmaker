package plan_test

import (
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"kultyaska/internal/plan"
)

var update = flag.Bool("update", false, "перезаписать эталонные файлы")

// presetPath — каркас, на котором строятся эталоны.
const presetPath = "../web/presets/08-zaskoruzlye-piramidy.json"

// fixturePlan собирает детерминированный план: он же используется
// в node-тесте tests/engine.test.js, чтобы JS и Go считали одинаково.
func fixturePlan(t *testing.T) *plan.Plan {
	t.Helper()
	raw, err := os.ReadFile(presetPath)
	if err != nil {
		t.Fatalf("каркас: %v", err)
	}
	sk, err := plan.ParseSkeleton(raw)
	if err != nil {
		t.Fatalf("разбор каркаса: %v", err)
	}
	p := sk.NewPlan("user-1", "План «всё влезет»", "🔥")
	p.Picks = []plan.Pick{
		{Key: "patch:otkluchenie_trenirovok"},
		{Key: "lesson:velikiy_uspeh"},
		{Key: "lesson:uchene_svet"},
		{Key: "lesson:prirozhdennyy_alhimik"},
		{Key: "lesson:da_sdohni_zhe"},
		{Key: "lesson:professionalnaya_deva", Qty: 1},
		{Key: "lesson:posledniy_boy"},
		// «поцелуй Мелисандры» не перечисляем: его Ли Лун уже купила сама,
		// и Normalize сам поставит его первым уроком.
		{Key: "lesson:ping_sistem"},
		{Key: "lesson:metod_neproglyadnogo_tumana"},
		{Key: "item:bazovoe_vyzhivanie", Qty: 1},
		{Key: "item:uchenica_kopiya", Qty: 1},
		{Key: "item:lestnica_v_nebo", Qty: 1},
	}
	p.Deferred = []plan.Deferred{{Key: "lesson:metod_ognennogo_vodopada", Slot: 1}}
	p.NotTaken = []string{"lesson:povyshenie_chistoty_korney", "lesson:nenavizhu_yady"}
	p.Folios = []plan.Folio{{Key: "folio:metod_neproglyadnogo_tumana", Name: "метод Непроглядного Тумана", Take: true, Cost: "50 XP"}}
	for i := range p.Accumulators {
		if p.Accumulators[i].Name == "горожане" {
			p.Accumulators[i].Activate = true
		}
	}
	set := func(key string, v float64) {
		if l := p.Ladder(key); l != nil {
			l.Value = v
		}
	}
	set("talent.die", 9)
	set("talent.dice_count", 3)
	set("talent.flat", 1)
	set("roots.max", 10)
	set("roots.min", 3)
	set("chance.billion", 1)
	p.Options = plan.Options{Decimals: true, Numbered: true}
	p.Normalize()
	return p
}

// TestExportGolden сверяет текст плана с эталоном и заодно обновляет
// фикстуры для node-теста JS-движка.
func TestExportGolden(t *testing.T) {
	p := fixturePlan(t)
	sum := plan.Compute(p)
	got := plan.Render(p, sum)

	dir := filepath.Join("testdata")
	planFile := filepath.Join(dir, "plan.json")
	goldenFile := filepath.Join(dir, "export.golden")
	sectionsFile := filepath.Join(dir, "sections.json")

	if *update {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		writeJSON(t, planFile, p)
		writeJSON(t, sectionsFile, plan.Sections)
		if err := os.WriteFile(goldenFile, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		t.Log("эталоны обновлены")
		return
	}

	want, err := os.ReadFile(goldenFile)
	if err != nil {
		t.Fatalf("нет эталона (%v); запусти: go test ./internal/plan -update", err)
	}
	if string(want) != got {
		t.Errorf("текст плана разошёлся с эталоном.\n--- получено ---\n%s\n--- ожидалось ---\n%s", got, string(want))
	}
}

func writeJSON(t *testing.T, path string, v any) {
	t.Helper()
	raw, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(raw, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestLessonCost(t *testing.T) {
	cases := map[int]float64{0: 0, 1: 5, 2: 10, 3: 15, 10: 50}
	for n, want := range cases {
		if got := plan.LessonCost(n); got != want {
			t.Errorf("урок %d: получил %v, ждал %v", n, got, want)
		}
	}
}

func TestLadderCosts(t *testing.T) {
	ladders := plan.DefaultLadders()
	byKey := map[string]plan.LadderCfg{}
	for _, l := range ladders {
		byKey[l.Key] = l
	}
	cases := []struct {
		key   string
		value float64
		want  float64
	}{
		{"talent.die", 1, 0},          // талант 1 — стартовое значение
		{"talent.die", 3, 2 + 3},      // d2 + d3
		{"talent.dice_count", 3, 10 + 20},
		{"talent.flat", 2, 5 + 10},
		{"roots.max", 10, 10},         // два шага по 5
		{"roots.min", 3, 1 + 2 + 3},   // проценты дорожают
		{"chance.billion", 2, 1 + 10},
		{"chance.million_home", 3, 50 + 100 + 150},
	}
	for _, c := range cases {
		l := byKey[c.key]
		l.Value = c.value
		if got := l.ValueCost(); got != c.want {
			t.Errorf("%s=%v: получил %v XP, ждал %v", c.key, c.value, got, c.want)
		}
	}
}

func TestSummaryWarningsAndSections(t *testing.T) {
	p := fixturePlan(t)
	sum := plan.Compute(p)

	if len(sum.Sections) == 0 {
		t.Fatal("нет ни одной секции")
	}
	// Остаток считается последовательно по секциям.
	run := p.Balance
	for _, sec := range sum.Sections {
		run -= sec.Sum
		if diff := run - sec.Remainder; diff > 1e-9 || diff < -1e-9 {
			t.Errorf("секция %s: остаток %v, ждал %v", sec.Key, sec.Remainder, run)
		}
	}
	if sum.Total <= 0 {
		t.Error("сумма плана должна быть положительной")
	}
	// «пинг систем» требует минимум 10-й урок, а уроков в фикстуре меньше → предупреждение.
	found := false
	for _, w := range sum.Warnings {
		if strings.Contains(w, "пинг систем") || strings.Contains(w, "минимум") {
			found = true
		}
	}
	if !found {
		t.Errorf("ждал предупреждение про «минимум n-ным уроком», получил %v", sum.Warnings)
	}
	if sum.DeferredLimit < 1 {
		t.Error("слотов невыученных должно быть минимум один")
	}
	if len(sum.NotTaken) == 0 {
		t.Error("не взятые уроки не попали в итог")
	}
}

func TestParseSkeletonValidation(t *testing.T) {
	if _, err := plan.ParseSkeleton([]byte(`{"name":"пусто","shop":[]}`)); err == nil {
		t.Error("пустой каркас должен отвергаться")
	}
	sk, err := plan.ParseSkeleton([]byte(`{"name":"тест","balance":10,"shop":[{"title":"урок","kind":"lesson"}]}`))
	if err != nil {
		t.Fatalf("каркас не разобрался: %v", err)
	}
	if sk.Shop[0].Key == "" || sk.Ladders == nil {
		t.Error("Normalize должен заполнить ключ и шкалы по умолчанию")
	}
	if sk.Shop[0].Fixed {
		t.Error("урок без цены не должен быть фиксированным")
	}
}

func TestEmptyPlanCostsNothing(t *testing.T) {
	p := plan.EmptyPlan("u", "пустой", "", 100)
	p.Picks = []plan.Pick{{Key: "нет-такого"}}
	sum := plan.Compute(p)
	if sum.Total != 0 {
		t.Errorf("несуществующая позиция не должна стоить ничего, получил %v", sum.Total)
	}
}

// TestLockedAndRequirements — позиции, которые Ли Лун уже взяла сама,
// и условия каркаса: план не должен им противоречить.
func TestLockedAndRequirements(t *testing.T) {
	raw, err := os.ReadFile(presetPath)
	if err != nil {
		t.Fatal(err)
	}
	sk, err := plan.ParseSkeleton(raw)
	if err != nil {
		t.Fatal(err)
	}

	p := sk.NewPlan("u1", "Проверка", "")
	if !p.Picked("lesson:poceluy_melisandry") {
		t.Error("уже купленный ею урок должен быть в плане")
	}
	if !p.Picked("lesson:ping_sistem") {
		t.Error("обязательный урок должен быть в плане")
	}
	if !p.IsLocked("lesson:ping_sistem") {
		t.Error("позиция помечена как обязательная")
	}

	// Уже взятая ею заплатка тоже обязательна и помечена в плане.
	if !p.Picked("patch:otkluchenie_trenirovok") || !p.IsLocked("patch:otkluchenie_trenirovok") {
		t.Error("«отключение тренировок» Ли Лун уже взяла — позиция обязана быть в плане")
	}

	// Уже купленный урок занимает первый номер, а опыт за него платим мы.
	sum := plan.Compute(p)
	lessons := sectionByKey(t, sum, plan.SecLessons)
	first := lessons.Lines[0]
	if first.Number != 1 || first.Cost != plan.LessonCost(1) {
		t.Errorf("первый урок: номер %d, цена %v — ждал 1 и %v", first.Number, first.Cost, plan.LessonCost(1))
	}
	if !strings.Contains(first.Note, "уже куплено") {
		t.Errorf("у первого урока нет пометки «уже куплено»: %q", first.Note)
	}
	second := lessons.Lines[1]
	if second.Number != 2 || second.Cost != plan.LessonCost(2) {
		t.Errorf("второй урок: номер %d, цена %v — ждал 2 и %v", second.Number, second.Cost, plan.LessonCost(2))
	}

	// Условие про Джона Сноу: пока не взяты ни «контроль пола», ни фолиант — предупреждение.
	if !hasWarning(sum.Warnings, "Джона Сноу") {
		t.Errorf("ждал предупреждение об условии каркаса, получил %v", sum.Warnings)
	}
	p.Picks = append(p.Picks, plan.Pick{Key: "item:vybor_pola"})
	p.Normalize()
	if hasWarning(plan.Compute(p).Warnings, "Джона Сноу") {
		t.Error("после «контроля пола» условие должно выполняться")
	}

	// Обязательную позицию нельзя отложить невыученными уроками.
	p.Deferred = []plan.Deferred{{Key: "lesson:ping_sistem", Slot: 1}}
	p.Normalize()
	if p.IsDeferred("lesson:ping_sistem") {
		t.Error("обязательную позицию нельзя отложить")
	}

	// Если обязательную позицию всё же убрать — конструктор обязан предупредить.
	p2 := sk.NewPlan("u2", "Без пинга", "")
	kept := p2.Picks[:0]
	for _, pk := range p2.Picks {
		if pk.Key != "lesson:ping_sistem" {
			kept = append(kept, pk)
		}
	}
	p2.Picks = kept
	if !hasWarning(plan.Compute(p2).Warnings, "обязательная позиция «пинг систем»") {
		t.Errorf("ждал предупреждение об отсутствующей обязательной позиции, получил %v", plan.Compute(p2).Warnings)
	}
}

func sectionByKey(t *testing.T, sum *plan.Summary, key string) plan.SectionSummary {
	t.Helper()
	for _, s := range sum.Sections {
		if s.Key == key {
			return s
		}
	}
	t.Fatalf("нет секции %s", key)
	return plan.SectionSummary{}
}

func hasWarning(list []string, sub string) bool {
	for _, w := range list {
		if strings.Contains(w, sub) {
			return true
		}
	}
	return false
}
