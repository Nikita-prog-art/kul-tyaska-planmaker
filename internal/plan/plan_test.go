package plan_test

import (
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"strconv"
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
		{Key: "item:ponaslyshke_mech", Qty: 1},
		{Key: "item:hardkor", Qty: 1},
		{Key: "folio:vzyat_1_foliant", Qty: 1},
		{Key: "item:pererozhdenie_dzhona", Qty: 1},
	}
	p.Deferred = []plan.Deferred{{Key: "lesson:metod_ognennogo_vodopada", Slot: 1}}
	p.NotTaken = []string{"lesson:povyshenie_chistoty_korney", "lesson:nenavizhu_yady"}
	p.Folios = []plan.Folio{{Key: "folio:metod_neproglyadnogo_tumana", Name: "метод Непроглядного Тумана", Take: true, Cost: "50 XP"}}
	for i := range p.Accumulators {
		if p.Accumulators[i].Name == "горожане" || p.Accumulators[i].Name == "хардкор" {
			p.Accumulators[i].Activate = true
		}
	}
	set := func(key string, v float64) {
		if l := p.Ladder(key); l != nil {
			l.Value = v
		}
	}
	set("talent.die", 24)
	set("talent.dice_count", 5)
	set("talent.flat", 4)
	set("roots.min", 50)
	set("chance.billion", 3)
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
		{"talent.die", 1, 0},     // талант 1 — стартовое значение
		{"talent.die", 3, 2 + 3}, // d2 + d3
		{"talent.dice_count", 3, 10 + 20},
		{"talent.flat", 2, 5 + 10},
		{"roots.max", 10, 10},       // два шага по 5
		{"roots.min", 3, 1 + 2 + 3}, // проценты дорожают
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

	// Условие про Джона Сноу — памятка «на будущие жизни», а не проверка.
	if hasWarning(sum.Warnings, "Джона Сноу") {
		t.Errorf("условие на будущее не должно попадать в проверки: %v", sum.Warnings)
	}
	if len(sum.Future) != 1 || sum.Future[0].Met {
		t.Fatalf("ждал одно невыполненное условие на будущее, получил %+v", sum.Future)
	}
	if !strings.Contains(sum.Future[0].Text, "Джона Сноу") {
		t.Errorf("в памятке нет условия про Джона Сноу: %q", sum.Future[0].Text)
	}
	p.Picks = append(p.Picks, plan.Pick{Key: "item:vybor_pola"})
	p.Normalize()
	if fut := plan.Compute(p).Future; len(fut) != 1 || !fut[0].Met {
		t.Errorf("после «контроля пола» условие должно быть выполнено: %+v", fut)
	}

	// Награда за порядок: «последний бой» после «пинг систем».
	// «Пинг систем» обязателен и приезжает сам, поэтому ставим «последний бой»
	// перед ним — награда теряется.
	p3 := sk.NewPlan("u3", "Награда", "")
	p3.Picks = []plan.Pick{{Key: "lesson:posledniy_boy"}}
	p3.Normalize()
	lost := plan.Compute(p3)
	if !hasWarning(lost.Warnings, "квестовая награда не получена") {
		t.Errorf("ждал предупреждение о потерянной награде, получил %v", lost.Warnings)
	}
	// Ставим «последний бой» сразу после «пинг систем» — награда достаётся.
	p4 := sk.NewPlan("u4", "Награда", "")
	p4.Picks = []plan.Pick{{Key: "lesson:ping_sistem"}, {Key: "lesson:posledniy_boy"}}
	p4.Normalize()
	got := plan.Compute(p4)
	if hasWarning(got.Warnings, "квестовая награда") {
		t.Errorf("награда не должна теряться: %v", got.Warnings)
	}
	if len(got.Rewards) != 1 || !strings.Contains(got.Rewards[0], "последний бой") {
		t.Fatalf("ждал одну награду за порядок, получил %+v", got.Rewards)
	}
	txt := plan.Render(p4, got)
	if !strings.Contains(txt, "квестовая награда") {
		t.Error("в тексте плана нет пометки о квестовой награде")
	}
	if !strings.Contains(txt, "⚠ Проверки не выполняются") {
		t.Error("в тексте плана нет предупреждения о проваленных проверках")
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

// TestPlansFromPlayers — планы игроков на жизнь 8 (их проверял ГМ) в том виде,
// в каком они сданы. Если план не сходится с каркасом — ошибка в каркасе, а не
// в плане. Каждый случай повторяет чужой план покупка в покупку и сверяет
// остаток XP с тем, что написал игрок. Формат позиции: «ключ» или «ключ*N»
// (взять N раз).
func TestPlansFromPlayers(t *testing.T) {
	raw, err := os.ReadFile(presetPath)
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name    string
		comment string
		picks   []string
		ladders map[string]float64
		want    float64
	}{
		{
			name:    "Не жадничаем",
			comment: "план: 133,24148 XP",
			picks: []string{
				// уроки 2..11 (первый — уже купленный «поцелуй Мелисандры»)
				"lesson:maksimum_zaradov_gorozhan",
				"lesson:skorost_zaradki_gorozhan",
				"lesson:vodnoe_teloslozhenie",
				"lesson:metod_neproglyadnogo_tumana",
				"lesson:prirozhdennyy_alhimik",
				"lesson:sama_sebe_bezoar",
				"lesson:da_sdohni_zhe",
				"lesson:rodoslovnaya_raduzhnoy_foreli",
				"lesson:ping_sistem",
				"lesson:posledniy_boy",
				// фиксированные уроки: 5 XP и трофей за 0
				"lesson:professionalnaya_deva",
				"lesson:metod_yadovityh_miazmov",
				// тренировки: 39 XP
				"item:ponaslyshke_mech", "item:bazovoe_vyzhivanie", "item:uchenica_kopiya",
				"item:diletantka_schita", "item:uchenica_alhimika", "item:lestnica_v_nebo*2",
				// заплатки: 30 XP, накопители: 40 XP
				"patch:bliznecy", "patch:uslovnyy_otstoy",
				"item:hardkor", "item:ravnovesie_zla_i_dobra",
				// библиотека: право взять фолиант и выкуп метода
				"folio:vzyat_1_foliant", "folio:vykupit_tuman",
			},
			ladders: map[string]float64{
				"talent.die": 25, "talent.dice_count": 5, "talent.flat": 6,
				"roots.min": 50, "chance.billion": 3, "chance.million_home": 1,
			},
			want: 133.24148,
		},
		{
			name:    "Компромисс",
			comment: "план: 21 XP, в плане арифметическая опечатка",
			picks: []string{
				"lesson:maksimum_zaradov_gorozhan",
				"lesson:skorost_zaradki_gorozhan",
				"lesson:vodnoe_teloslozhenie",
				"lesson:metod_neproglyadnogo_tumana",
				"lesson:prirozhdennyy_alhimik",
				"lesson:sama_sebe_bezoar",
				"lesson:da_sdohni_zhe",
				"lesson:nenavizhu_yady",
				"lesson:ping_sistem",
				"lesson:posledniy_boy",
				"lesson:professionalnaya_deva", "lesson:metod_yadovityh_miazmov",
				"item:bazovoe_vyzhivanie", "item:uchenica_kopiya", "item:diletantka_schita",
				"item:uchenica_alhimika", "item:lestnica_v_nebo",
				"item:hardkor",
				"folio:vzyat_1_foliant", "folio:vykupit_tuman",
			},
			ladders: map[string]float64{"talent.flat": 5, "roots.min": 70},
			want:    20.24148,
		},
		{
			name:    "Берем всё",
			comment: "план: 295 XP",
			picks: []string{
				"lesson:maksimum_zaradov_gorozhan",
				"lesson:skorost_zaradki_gorozhan",
				"lesson:vodnoe_teloslozhenie",
				"lesson:metod_neproglyadnogo_tumana",
				"lesson:prirozhdennyy_alhimik",
				"lesson:sama_sebe_bezoar",
				"lesson:da_sdohni_zhe",
				"lesson:nenavizhu_yady",
				"lesson:ping_sistem",
				"lesson:uchene_svet",
				"lesson:diletantka_shirokogo_profilya",
				"lesson:professionalnaya_deva",
				"lesson:metod_yadovityh_miazmov", "lesson:metod_tonkogo_pogloscheniya",
				"item:bazovoe_vyzhivanie", "item:uchenica_kopiya", "item:diletantka_schita",
				"item:uchenica_alhimika", "item:lestnica_v_nebo",
				"item:hardkor",
				"folio:vykupit_tuman", "folio:vzyat_1_foliant", "folio:vzyat_posle_pola",
			},
			ladders: map[string]float64{
				"talent.flat": 4, "roots.min": 50,
				"chance.billion": 3, "chance.million_home": 1,
			},
			want: 295.24148,
		},
		{
			name:    "Жаждя Большего",
			comment: "план: 201 XP",
			picks: []string{
				"lesson:metod_neproglyadnogo_tumana",
				"lesson:skorost_zaradki_gorozhan",
				"lesson:maksimum_zaradov_gorozhan",
				"lesson:vodnoe_teloslozhenie",
				"lesson:uchene_svet",
				"lesson:prirozhdennyy_alhimik",
				"lesson:sama_sebe_bezoar",
				"lesson:da_sdohni_zhe",
				"lesson:prodlenie_bitvy",
				"lesson:ping_sistem",
				"lesson:posledniy_boy",
				"lesson:professionalnaya_deva",
				"item:bazovoe_vyzhivanie", "item:uchenica_alhimika", "item:uchenica_kopiya",
				"item:ponaslyshke_mech", "item:lestnica_v_nebo*2",
				"folio:vzyat_1_foliant", "folio:vykupit_tuman", "folio:vzyat_posle_pola",
				"item:hardkor", "item:ravnovesie_zla_i_dobra",
				"patch:bliznecy",
			},
			ladders: map[string]float64{
				"talent.die": 25, "talent.dice_count": 5,
				"roots.min": 43, "chance.billion": 3, "chance.million_home": 2,
			},
			want: 201.24148,
		},
		{
			name:    "Песнь Таланта и Корней",
			comment: "план: 3 XP",
			picks: []string{
				"lesson:maksimum_zaradov_gorozhan",
				"lesson:skorost_zaradki_gorozhan",
				"lesson:uchene_svet",
				"lesson:vodnoe_teloslozhenie",
				"lesson:metod_neproglyadnogo_tumana",
				"lesson:diletantka_shirokogo_profilya",
				"lesson:prirozhdennyy_alhimik",
				"lesson:sama_sebe_bezoar",
				"lesson:da_sdohni_zhe",
				"lesson:nenavizhu_yady",
				"lesson:ping_sistem",
				"lesson:professionalnaya_deva", "lesson:metod_yadovityh_miazmov",
				"patch:bliznecy",
				"item:ponaslyshke_mech", "item:uchenica_kopiya", "item:diletantka_schita",
				"item:bazovoe_vyzhivanie", "item:uchenica_alhimika", "item:lestnica_v_nebo",
				"item:hardkor",
				"folio:vzyat_1_foliant", "folio:vzyat_posle_okraski",
				"folio:vzyat_posle_pola", "folio:vykupit_tuman",
				"item:pererozhdenie_dzhona",
			},
			ladders: map[string]float64{
				"talent.die": 25, "talent.dice_count": 5,
				"roots.min": 50, "chance.billion": 3,
			},
			want: 3.24148,
		},
		{
			name:    "Скромненько",
			comment: "план: 113 XP",
			picks: []string{
				"lesson:metod_neproglyadnogo_tumana",
				"lesson:skorost_zaradki_gorozhan",
				"lesson:maksimum_zaradov_gorozhan",
				"lesson:vodnoe_teloslozhenie",
				"lesson:prirozhdennyy_alhimik",
				"lesson:sama_sebe_bezoar",
				"lesson:da_sdohni_zhe",
				"lesson:nenavizhu_yady",
				"lesson:ping_sistem",
				"lesson:uchene_svet",
				"lesson:professionalnaya_deva", "lesson:metod_yadovityh_miazmov",
				"item:uchenica_alhimika", "item:bazovoe_vyzhivanie", "item:ponaslyshke_mech",
				"item:uchenica_kopiya", "item:diletantka_schita", "item:lestnica_v_nebo",
				"item:hardkor", "item:ravnovesie_zla_i_dobra",
				"item:pererozhdenie_dzhona",
			},
			ladders: map[string]float64{
				"talent.die": 25, "talent.dice_count": 5,
				"roots.min": 50, "chance.billion": 3, "chance.million_home": 1,
			},
			want: 113.24148,
		},
		{
			name:    "Шлакоблокунь",
			comment: "план: 142 XP",
			picks: []string{
				"lesson:maksimum_zaradov_gorozhan",
				"lesson:skorost_zaradki_gorozhan",
				"lesson:vodnoe_teloslozhenie",
				"lesson:metod_neproglyadnogo_tumana",
				"lesson:prirozhdennyy_alhimik",
				"lesson:sama_sebe_bezoar",
				"lesson:da_sdohni_zhe",
				"lesson:nenavizhu_yady",
				"lesson:ping_sistem",
				"lesson:prodlenie_bitvy",
				"lesson:diletantka_shirokogo_profilya",
				"lesson:professionalnaya_deva", "lesson:metod_yadovityh_miazmov",
				"item:bazovoe_vyzhivanie", "item:uchenica_alhimika",
				"item:ponaslyshke_mech", "item:lestnica_v_nebo",
				"item:hardkor",
				"folio:vykupit_tuman", "folio:vzyat_1_foliant",
				"item:pererozhdenie_dzhona",
			},
			ladders: map[string]float64{
				"talent.flat": 4, "roots.min": 50,
				"chance.billion": 3, "chance.million_home": 1,
			},
			want: 142.24148,
		},
		{
			name:    "Готовимся к жизни с Успехом и Джонни",
			comment: "план: 140 XP",
			picks: []string{
				"lesson:maksimum_zaradov_gorozhan",
				"lesson:skorost_zaradki_gorozhan",
				"lesson:vodnoe_teloslozhenie",
				"lesson:uchene_svet",
				"lesson:nenavizhu_yady",
				"lesson:diletantka_shirokogo_profilya",
				"lesson:prirozhdennyy_alhimik",
				"lesson:sama_sebe_bezoar",
				"lesson:ping_sistem",
				"lesson:da_sdohni_zhe",
				"lesson:posledniy_boy",
				"item:ponaslyshke_mech", "item:bazovoe_vyzhivanie", "item:uchenica_kopiya",
				"item:diletantka_schita", "item:uchenica_alhimika", "item:lestnica_v_nebo",
				"lesson:metod_yadovityh_miazmov",
				"patch:bliznecy", "item:ravnovesie_zla_i_dobra", "item:hardkor",
				"item:pererozhdenie_dzhona",
			},
			ladders: map[string]float64{
				"talent.flat": 5, "roots.min": 50,
				"chance.billion": 3, "chance.million_home": 1,
			},
			want: 140.24148,
		},
		{
			name:    "Друг детства",
			comment: "план: 38 XP",
			picks: []string{
				"lesson:maksimum_zaradov_gorozhan",
				"lesson:skorost_zaradki_gorozhan",
				"lesson:uchene_svet",
				"lesson:vodnoe_teloslozhenie",
				"lesson:metod_neproglyadnogo_tumana",
				"lesson:diletantka_shirokogo_profilya",
				"lesson:prirozhdennyy_alhimik",
				"lesson:sama_sebe_bezoar",
				"lesson:da_sdohni_zhe",
				"lesson:nenavizhu_yady",
				"lesson:ping_sistem",
				"lesson:professionalnaya_deva", "lesson:metod_yadovityh_miazmov",
				"item:ponaslyshke_mech", "item:uchenica_kopiya", "item:diletantka_schita",
				"item:bazovoe_vyzhivanie", "item:uchenica_alhimika", "item:lestnica_v_nebo",
				"folio:vzyat_1_foliant", "folio:vykupit_tuman",
				"item:pererozhdenie_dzhona",
			},
			ladders: map[string]float64{
				"talent.die": 25, "talent.dice_count": 5, "roots.min": 50,
				"chance.billion": 3, "chance.million_home": 1,
			},
			want: 38.24148,
		},
		{
			name:    "Фолианты и уроки",
			comment: "план: 68 XP",
			picks: []string{
				"lesson:maksimum_zaradov_gorozhan",
				"lesson:skorost_zaradki_gorozhan",
				"lesson:uchene_svet",
				"lesson:vodnoe_teloslozhenie",
				"lesson:prirozhdennyy_alhimik",
				"lesson:sama_sebe_bezoar",
				"lesson:diletantka_shirokogo_profilya",
				"lesson:da_sdohni_zhe",
				"lesson:nenavizhu_yady",
				"lesson:ping_sistem",
				"lesson:posledniy_boy",
				"lesson:metod_neproglyadnogo_tumana",
				"lesson:professionalnaya_deva",
				"lesson:metod_yadovityh_miazmov", "lesson:metod_tonkogo_pogloscheniya",
				"item:bazovoe_vyzhivanie", "item:uchenica_kopiya", "item:diletantka_schita",
				"item:uchenica_alhimika", "item:ponaslyshke_mech", "item:lestnica_v_nebo*2",
				"folio:vzyat_1_foliant*2", "folio:vzyat_posle_pola", "folio:vzyat_posle_okraski",
				"folio:vykupit_tuman", "folio:vykupit_miazmy",
				"item:hardkor", "item:ravnovesie_zla_i_dobra",
				"item:pererozhdenie_dzhona",
			},
			ladders: map[string]float64{
				"talent.die": 25, "talent.dice_count": 5, "roots.min": 50,
			},
			want: 68.24148,
		},
		{
			name:    "Всё нужное, ничего лишнего",
			comment: "план: 38,24148 XP",
			picks: []string{
				"lesson:maksimum_zaradov_gorozhan",
				"lesson:skorost_zaradki_gorozhan",
				"lesson:vodnoe_teloslozhenie",
				"lesson:uchene_svet",
				"lesson:nenavizhu_yady",
				"lesson:diletantka_shirokogo_profilya",
				"lesson:prirozhdennyy_alhimik",
				"lesson:sama_sebe_bezoar",
				"lesson:ping_sistem",
				"lesson:da_sdohni_zhe",
				"lesson:posledniy_boy",
				"lesson:prodlenie_bitvy",
				"lesson:professionalnaya_deva",
				"lesson:metod_yadovityh_miazmov",
				"item:bazovoe_vyzhivanie", "item:uchenica_kopiya", "item:diletantka_schita",
				"item:uchenica_alhimika", "item:ponaslyshke_mech", "item:lestnica_v_nebo",
				"item:hardkor", "item:ravnovesie_zla_i_dobra",
				"folio:vzyat_1_foliant", "folio:vzyat_posle_okraski",
				"folio:vykupit_miazmy",
				"item:pererozhdenie_dzhona",
			},
			ladders: map[string]float64{
				"talent.die": 25, "talent.dice_count": 5, "talent.flat": 6,
				"roots.min": 50, "chance.million_home": 1,
			},
			want: 38.24148,
		},
		{
			name:    "Ну раз просят",
			comment: "план: 38,24148 XP",
			picks: []string{
				"lesson:maksimum_zaradov_gorozhan",
				"lesson:skorost_zaradki_gorozhan",
				"lesson:vodnoe_teloslozhenie",
				"lesson:uchene_svet",
				"lesson:nenavizhu_yady",
				"lesson:diletantka_shirokogo_profilya",
				"lesson:prirozhdennyy_alhimik",
				"lesson:sama_sebe_bezoar",
				"lesson:ping_sistem",
				"lesson:da_sdohni_zhe",
				"lesson:metod_neproglyadnogo_tumana",
				"lesson:prodlenie_bitvy",
				"lesson:professionalnaya_deva",
				"lesson:metod_yadovityh_miazmov",
				"item:bazovoe_vyzhivanie", "item:uchenica_kopiya", "item:diletantka_schita",
				"item:uchenica_alhimika", "item:ponaslyshke_mech", "item:lestnica_v_nebo",
				"item:hardkor", "item:ravnovesie_zla_i_dobra",
				"folio:vzyat_1_foliant", "folio:vzyat_posle_okraski",
				"folio:vykupit_tuman",
				"item:pererozhdenie_dzhona",
			},
			ladders: map[string]float64{
				"talent.die": 25, "talent.dice_count": 5, "talent.flat": 6,
				"roots.min": 50, "chance.million_home": 1,
			},
			want: 38.24148,
		},
		{
			name:    "План протянутой руки дружбы",
			comment: "план: 158 XP",
			picks: []string{
				"lesson:maksimum_zaradov_gorozhan",
				"lesson:skorost_zaradki_gorozhan",
				"lesson:vodnoe_teloslozhenie",
				"lesson:uchene_svet",
				"lesson:nenavizhu_yady",
				"lesson:diletantka_shirokogo_profilya",
				"lesson:prirozhdennyy_alhimik",
				"lesson:sama_sebe_bezoar",
				"lesson:ping_sistem",
				"lesson:da_sdohni_zhe",
				"lesson:posledniy_boy",
				"item:ponaslyshke_mech", "item:bazovoe_vyzhivanie", "item:uchenica_kopiya",
				"item:diletantka_schita", "item:uchenica_alhimika", "item:lestnica_v_nebo",
				"lesson:metod_yadovityh_miazmov",
				"patch:bliznecy", "item:ravnovesie_zla_i_dobra", "item:hardkor",
				"item:pererozhdenie_dzhona",
			},
			ladders: map[string]float64{
				"talent.die": 25, "talent.dice_count": 5, "talent.flat": 6,
				"roots.min": 50, "chance.million_home": 1,
			},
			want: 158.24148,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			sk, err := plan.ParseSkeleton(raw)
			if err != nil {
				t.Fatal(err)
			}
			p := sk.NewPlan("u", c.name, "")
			p.Picks = nil
			seen := map[string]int{}
			for _, raw := range c.picks {
				key, qty := raw, 1
				if i := strings.Index(raw, "*"); i > 0 {
					key = raw[:i]
					if n, err := strconv.Atoi(raw[i+1:]); err == nil {
						qty = n
					}
				}
				if p.Item(key) == nil {
					t.Fatalf("в каркасе нет позиции %q", key)
				}
				if at, ok := seen[key]; ok {
					p.Picks[at].Qty = qty
					continue
				}
				seen[key] = len(p.Picks)
				p.Picks = append(p.Picks, plan.Pick{Key: key, Qty: qty})
			}
			for key, v := range c.ladders {
				l := p.Ladder(key)
				if l == nil {
					t.Fatalf("в каркасе нет шкалы %q", key)
				}
				l.Value = v
			}
			p.Folios = nil
			p.Accumulators = nil
			p.Normalize()
			got := plan.Compute(p)
			if len(got.Warnings) > 0 {
				t.Errorf("план должен сходиться без проверок, получил %v", got.Warnings)
			}
			if diff := got.Remainder - c.want; diff > 1e-6 || diff < -1e-6 {
				t.Errorf("%s: остаток %v, ждал %v (потрачено %v)",
					c.comment, got.Remainder, c.want, got.Total)
			}
		})
	}
}
