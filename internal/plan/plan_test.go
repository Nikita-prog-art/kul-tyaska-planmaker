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
const presetPath = "../web/presets/06-zaskoruzlye-piramidy.json"

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
		{Key: "patch:zerkalo_slavy"},
		{Key: "lesson:velikiy_uspeh"},
		{Key: "lesson:uchene_svet"},
		{Key: "lesson:prirozhdennyy_alhimik"},
		{Key: "lesson:da_sdohni_zhe"},
		{Key: "lesson:professionalnaya_deva", Qty: 1},
		{Key: "lesson:posledniy_boy"},
		{Key: "lesson:poceluy_melisandry"},
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
