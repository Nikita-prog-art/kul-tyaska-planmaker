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
		{Key: "folio:vykupit_tuman", Qty: 1},
		// Покупки, которые меняют накопитель: «максимум зарядов» поднимает
		// максимум «горожан» до 2, «скорость зарядки» — 20% до 25% за жизнь.
		{Key: "lesson:maksimum_zaradov_gorozhan", Qty: 1},
		{Key: "lesson:skorost_zaradki_gorozhan", Qty: 1},
		{Key: "item:pererozhdenie_dzhona", Qty: 1},
	}
	p.Deferred = []plan.Deferred{{Key: "lesson:metod_ognennogo_vodopada", Slot: 1}}
	p.NotTaken = []string{"lesson:povyshenie_chistoty_korney", "lesson:nenavizhu_yady"}
	// Фолианты берём из каркаса: у них уже есть ключи и условия (урок + выкуп).
	p.Folios[0].Take = true
	for i := range p.Accumulators {
		switch p.Accumulators[i].Name {
		case "хардкор":
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

// TestFixedLessonsHaveOwnSection — уроки с фиксированной ценой (трофеи и
// покупки за фиксированные XP) печатаются отдельным разделом: в общую
// нумерацию они не лезут, а в строке уроков им делать нечего.
func TestFixedLessonsHaveOwnSection(t *testing.T) {
	sk := presetSkeleton(t)
	p := sk.NewPlan("u", "Фиксированные", "")
	p.Picks = append(p.Picks, plan.Pick{Key: "lesson:professionalnaya_deva"})
	p.Normalize()
	sum := plan.Compute(p)

	var lessons, fixed *plan.SectionSummary
	for i := range sum.Sections {
		switch sum.Sections[i].Key {
		case plan.SecLessons:
			lessons = &sum.Sections[i]
		case plan.SecFixed:
			fixed = &sum.Sections[i]
		}
	}
	if lessons == nil || fixed == nil {
		t.Fatalf("ждал разделы уроков и фиксированных уроков, получил %+v", sum.Sections)
	}
	for _, l := range lessons.Lines {
		if l.Fixed {
			t.Errorf("фиксированный урок остался в общем разделе: %q", l.Title)
		}
	}
	found := false
	for _, l := range fixed.Lines {
		if l.Key == "lesson:professionalnaya_deva" {
			found = true
			if l.Number != 0 {
				t.Errorf("фиксированный урок получил номер %d", l.Number)
			}
		}
	}
	if !found {
		t.Error("«профессиональная Дева-в-Беде» не попала в свой раздел")
	}

	txt := plan.Render(p, sum)
	if !strings.Contains(txt, "Уроки с фиксированной ценой:") {
		t.Errorf("в тексте плана нет раздела фиксированных уроков:\n%s", txt)
	}
	// Уроки идут одной строкой: «Уроки: 1. … — 5, 2. … — 10».
	var lessonLines int
	for _, line := range strings.Split(txt, "\n") {
		if strings.HasPrefix(line, "Уроки: ") {
			lessonLines++
		}
	}
	if lessonLines != 1 {
		t.Errorf("ждал одну строку уроков, получил %d:\n%s", lessonLines, txt)
	}
	if !strings.Contains(txt, "Уроки с фиксированной ценой: Дева: 5") {
		t.Errorf("фиксированный урок не напечатан своей строкой:\n%s", txt)
	}
}

// TestLessonIgnoresCardSection — урок остаётся уроком, даже если каркас
// записал его в «Телосложения» или «Родословные»: нумерация, цена и раздел
// «Уроки» от этого не меняются.
func TestLessonIgnoresCardSection(t *testing.T) {
	sk := presetSkeleton(t)
	p := sk.NewPlan("u", "Телосложение", "")
	// Так выглядел старый каркас: урок лежал в чужой секции.
	for i := range p.Shop {
		switch p.Shop[i].Key {
		case "lesson:vodnoe_teloslozhenie":
			p.Shop[i].Section = plan.SecPhysique
		case "lesson:rodoslovnaya_raduzhnoy_foreli":
			p.Shop[i].Section = plan.SecBloodline
		}
	}
	p.Picks = append(p.Picks,
		plan.Pick{Key: "lesson:vodnoe_teloslozhenie"},
		plan.Pick{Key: "lesson:rodoslovnaya_raduzhnoy_foreli"},
	)
	p.Normalize()
	sum := plan.Compute(p)

	found := map[string]int{}
	for i := range sum.Sections {
		sec := &sum.Sections[i]
		switch sec.Key {
		case plan.SecPhysique, plan.SecBloodline:
			t.Errorf("уроки попали в раздел %q", sec.Key)
		}
		for _, l := range sec.Lines {
			if l.Key == "lesson:vodnoe_teloslozhenie" || l.Key == "lesson:rodoslovnaya_raduzhnoy_foreli" {
				found[l.Key] = sec.Order
				if sec.Key != plan.SecLessons {
					t.Errorf("урок %q оказался в разделе %q", l.Key, sec.Key)
				}
				if l.Number == 0 {
					t.Errorf("урок %q остался без номера", l.Key)
				}
			}
		}
	}
	for _, key := range []string{"lesson:vodnoe_teloslozhenie", "lesson:rodoslovnaya_raduzhnoy_foreli"} {
		if _, ok := found[key]; !ok {
			t.Errorf("урок %q не попал в план", key)
		}
	}
	txt := plan.Render(p, sum)
	line := ""
	for _, l := range strings.Split(txt, "\n") {
		if strings.HasPrefix(l, "Уроки: ") {
			line = l
		}
	}
	if !strings.Contains(line, "водное тело") || !strings.Contains(line, "форель") {
		t.Errorf("в строке уроков нет уроков из чужих секций:\n%s", txt)
	}
}

// TestDuplicateLadderItemDropped — покупка, которая повторяет шкалу
// («шанс «один на миллиард»» был и ступенью, и галочкой), из плана уходит:
// иначе одну позицию можно было взять дважды за 400 XP.
func TestDuplicateLadderItemDropped(t *testing.T) {
	sk := presetSkeleton(t)
	p := sk.NewPlan("u", "Миллиард", "")
	// Так выглядел план, сохранённый до правки каркаса.
	p.Shop = append(p.Shop, plan.ShopItem{
		Key: "chance:odin_na_milliard", Kind: plan.KindItem, Section: plan.SecChances,
		Title: "шанс «один на миллиард»", Cost: 200, Fixed: true,
	})
	p.Picks = append(p.Picks, plan.Pick{Key: "chance:odin_na_milliard"})
	if l := p.Ladder("chance.billion"); l != nil {
		l.Value = 3
	}
	p.Normalize()

	if p.Item("chance:odin_na_milliard") != nil {
		t.Error("покупка-дубль осталась в магазине")
	}
	for _, pk := range p.Picks {
		if pk.Key == "chance:odin_na_milliard" {
			t.Error("выбор покупки-дубля остался в плане")
		}
	}
	var chances *plan.SectionSummary
	sum := plan.Compute(p)
	for i := range sum.Sections {
		if sum.Sections[i].Key == plan.SecChances {
			chances = &sum.Sections[i]
		}
	}
	if chances == nil {
		t.Fatal("раздел «Шансы» пропал: ступень должна была остаться")
	}
	if len(chances.Lines) != 1 || chances.Lines[0].Key != "chance.billion" {
		t.Fatalf("в «Шансах» ждал одну ступень, получил %+v", chances.Lines)
	}
	if chances.Sum != 200 {
		t.Errorf("ступень стоит 200 XP, а не %v", chances.Sum)
	}
}

// TestExportListsOnlyTaken — «не активирую» и «оставляю в библиотеке»
// в план не печатаются: список того, что не берём, только занимает место.
func TestExportListsOnlyTaken(t *testing.T) {
	sk := presetSkeleton(t)
	p := sk.NewPlan("u", "Тихий", "")
	sum := plan.Compute(p)
	txt := plan.Render(p, sum)
	if strings.Contains(txt, "не активирую") {
		t.Errorf("в плане остался список «не активирую»:\n%s", txt)
	}
	if strings.Contains(txt, "оставляю в библиотеке") {
		t.Errorf("в плане остался список «оставляю в библиотеке»:\n%s", txt)
	}
	if strings.Contains(txt, "Активация накопителей:") {
		t.Error("блок активации напечатан, хотя активировать нечего")
	}
}

// TestExportOneLinePerSection — каждый раздел печатается одной строкой:
// «Заплатки: без тренировок: 0», а не списком по позиции на строку.
func TestExportOneLinePerSection(t *testing.T) {
	p := fixturePlan(t)
	sum := plan.Compute(p)
	txt := plan.Render(p, sum)

	for _, line := range strings.Split(txt, "\n") {
		if strings.HasPrefix(line, "  - ") {
			t.Fatalf("позиция напечатана отдельной строкой: %q\n%s", line, txt)
		}
	}
	for _, sec := range sum.Sections {
		var lines []string
		for _, line := range strings.Split(txt, "\n") {
			if strings.HasPrefix(line, sec.Title+": ") || line == sec.Title+":" {
				lines = append(lines, line)
			}
		}
		if len(lines) != 1 {
			t.Errorf("раздел %q напечатан %d строками, ждал одну: %q", sec.Title, len(lines), lines)
		}
	}
}

// TestExportUsesShortNames — в выгрузке позиции называются коротко:
// «1. Мелисандра: 5», а не «1. поцелуй Мелисандры: 5».
func TestExportUsesShortNames(t *testing.T) {
	p := fixturePlan(t)
	txt := plan.Render(p, plan.Compute(p))

	for _, want := range []string{"1. Мелисандра: 5", "Туман: 35", "Джон Сноу: 200", "корни мин 50%: 481", "Беру с собой: Туман"} {
		if !strings.Contains(txt, want) {
			t.Errorf("в выгрузке нет %q:\n%s", want, txt)
		}
	}
	for _, unwanted := range []string{"поцелуй Мелисандры", "метод Непроглядного Тумана (освоено)", "оплата перерождения Джона Сноу"} {
		if strings.Contains(txt, unwanted) {
			t.Errorf("в выгрузке осталось длинное название %q:\n%s", unwanted, txt)
		}
	}
}

// TestExportKeepsOnlyThePlan — заметки каркаса («Каркас по посмертию…»)
// и памятка «На будущие жизни» в план не попадают: план читают как список
// покупок, а памятка живёт в конструкторе.
func TestExportKeepsOnlyThePlan(t *testing.T) {
	p := fixturePlan(t)
	p.Notes = "Каркас по посмертию после подвига в секте Звенящих ручьёв"
	p.Normalize()
	sum := plan.Compute(p)
	if len(sum.Future) == 0 {
		t.Fatal("в каркасе нет условий «на будущие жизни» — проверять нечего")
	}
	txt := plan.Render(p, sum)
	if strings.Contains(txt, "На будущие жизни") {
		t.Errorf("в плане осталась памятка «На будущие жизни»:\n%s", txt)
	}
	if strings.Contains(txt, "Каркас по посмертию") {
		t.Errorf("в план попали заметки каркаса:\n%s", txt)
	}
	if strings.Contains(txt, "Джона Сноу") {
		t.Errorf("в плане осталось условие «на будущие жизни»:\n%s", txt)
	}
}

// TestExportHasNoSoftLineBreaks — переносы строк делает читатель (чат или
// страница), в самом тексте их нет: длинный раздел остаётся одной строкой.
func TestExportHasNoSoftLineBreaks(t *testing.T) {
	p := fixturePlan(t)
	txt := plan.Render(p, plan.Compute(p))
	for _, line := range strings.Split(txt, "\n") {
		if strings.HasSuffix(line, ",") {
			t.Fatalf("строка обрывается на запятой: %q", line)
		}
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
			t.Logf("потрачено %v, остаток %v (%s)", got.Total, got.Remainder, c.comment)
			if diff := got.Remainder - c.want; diff > 1e-6 || diff < -1e-6 {
				t.Errorf("%s: остаток %v, ждал %v (потрачено %v)",
					c.comment, got.Remainder, c.want, got.Total)
			}
		})
	}
}

// presetSkeleton читает каркас жизни 8 для тестов.
func presetSkeleton(t *testing.T) *plan.Skeleton {
	t.Helper()
	raw, err := os.ReadFile(presetPath)
	if err != nil {
		t.Fatalf("каркас: %v", err)
	}
	sk, err := plan.ParseSkeleton(raw)
	if err != nil {
		t.Fatalf("разбор каркаса: %v", err)
	}
	return sk
}

// TestAttachmentGate — отметки «активирую» и «беру» план обязан заслужить:
// накопитель — целым зарядом (или покупкой), фолиант — уроком метода и выкупом.
func TestAttachmentGate(t *testing.T) {
	sk := presetSkeleton(t)
	p := sk.NewPlan("u", "проверка отметок", "")

	// В свежем плане тратить нечего: «горожане» набрали 0,8 заряда из 1 —
	// прибавка за жизнь достанется в следующей жизни, — а «хардкор»,
	// «равновесие», «окрестности Му» не заряжены, «великий успех» набрал
	// половину заряда и в этом посмертии поднять его нечем: уроки на заряд
	// и максимум заряда остались в прошлых посмертиях и сюда не переносятся.
	ready := map[string]bool{}
	for i := range p.Accumulators {
		a := &p.Accumulators[i]
		st := p.AccumulatorState(a)
		if st.Ready != ready[a.Name] {
			t.Errorf("накопитель %q: доступен=%v (%s), ждал %v", a.Name, st.Ready, st.Reason, ready[a.Name])
		}
		if !st.Ready && st.Reason == "" {
			t.Errorf("накопитель %q недоступен без объяснения", a.Name)
		}
	}
	if st := p.AccumulatorState(&p.Accumulators[0]); !strings.Contains(st.Reason, "нет целого заряда: 0,8") {
		t.Errorf("про «горожане» ждал причину про 0,8 заряда, получил %q", st.Reason)
	}
	for i := range p.Accumulators {
		if p.Accumulators[i].Name != "великий успех" {
			continue
		}
		if st := p.AccumulatorState(&p.Accumulators[i]); !strings.Contains(st.Reason, "нет целого заряда: 0,5") {
			t.Errorf("«великий успех» ждал причину про 0,5 заряда, получил %q", st.Reason)
		}
	}
	if p.Item("lesson:zarad_velikogo_uspeha") != nil || p.Item("lesson:velikiy_uspeh") != nil {
		t.Error("уроки на заряд «великого успеха» не должны предлагаться в этом посмертии")
	}

	// Заряд добавляют покупки: купленный «хардкор» приходит уже заряженным.
	p.Picks = append(p.Picks, plan.Pick{Key: "item:hardkor"})
	p.Normalize()
	for i := range p.Accumulators {
		a := &p.Accumulators[i]
		if a.Name != "хардкор" {
			continue
		}
		if st := p.AccumulatorState(a); !st.Ready {
			t.Errorf("накопитель %q должен быть готов: %s", a.Name, st.Reason)
		}
	}
	// «горожане» покупками не заряжаются: 0,8 так и остаётся 0,8.
	if _, max, _ := p.AccumulatorValues(&p.Accumulators[0]); max != 1 {
		t.Errorf("максимум «горожан» без урока — 1, получил %v", max)
	}
	p.Picks = append(p.Picks,
		plan.Pick{Key: "lesson:maksimum_zaradov_gorozhan"},
		plan.Pick{Key: "lesson:skorost_zaradki_gorozhan"},
	)
	p.Normalize()
	charges, max, rate := p.AccumulatorValues(&p.Accumulators[0])
	if charges != 0.8 || max != 2 || rate != 25 {
		t.Errorf("«максимум зарядов» даёт 2, «скорость зарядки» — 25%%: %v/%v +%v%%", charges, max, rate)
	}
	if st := p.AccumulatorState(&p.Accumulators[0]); st.Ready {
		t.Error("«горожане» на 0,8 из 2 всё ещё не заряд — активировать нельзя")
	}
	// Накопитель на 1 из 2 уже можно тратить: заряд тратится по одному.
	for i := range p.Accumulators {
		if p.Accumulators[i].Name == "горожане" {
			p.Accumulators[i].Charges = 1
		}
	}
	p.Normalize()
	for i := range p.Accumulators {
		a := &p.Accumulators[i]
		if a.Name == "горожане" {
			if st := p.AccumulatorState(a); !st.Ready {
				t.Errorf("1 заряд из 2 (с покупкой максимума) — уже можно тратить: %s", st.Reason)
			}
		}
	}

	// Фолиант без урока метода и выкупа взять нельзя, и в подсказке есть оба.
	f := p.Folios[0]
	if st := p.FolioState(&f); st.Ready || !strings.Contains(st.Reason, "урок") || !strings.Contains(st.Reason, "выкуп") {
		t.Errorf("фолиант без урока и выкупа: %+v", st)
	}

	// Незаслуженные отметки Normalize снимает: сохранённый план сходится сам с собой.
	for i := range p.Accumulators {
		p.Accumulators[i].Activate = true
	}
	p.Folios[0].Take = true
	p.Normalize()
	for i := range p.Accumulators {
		a := &p.Accumulators[i]
		want := a.Name == "горожане" || a.Name == "хардкор"
		if a.Activate != want {
			t.Errorf("после Normalize накопитель %q: активирую=%v, ждал %v", a.Name, a.Activate, want)
		}
	}
	if p.Folios[0].Take {
		t.Error("после Normalize невыкупленный фолиант остался отмеченным")
	}

	// Урок метода и выкуп открывают фолиант.
	p.Picks = append(p.Picks,
		plan.Pick{Key: "lesson:metod_neproglyadnogo_tumana"},
		plan.Pick{Key: "folio:vykupit_tuman"},
	)
	p.Normalize()
	if st := p.FolioState(&p.Folios[0]); !st.Ready {
		t.Errorf("фолиант с уроком и выкупом недоступен: %s", st.Reason)
	}
	for i := range p.Accumulators {
		a := &p.Accumulators[i]
		if a.Name == "хардкор" {
			if st := p.AccumulatorState(a); !st.Ready {
				t.Errorf("купленный хардкор недоступен: %s", st.Reason)
			}
		}
	}

	// Выкуп без урока метода — ошибка в плане, о ней надо сказать.
	p.Picks = []plan.Pick{{Key: "lesson:poceluy_melisandry"}, {Key: "folio:vykupit_tuman"}}
	sum := plan.Compute(p)
	found := false
	for _, w := range sum.Warnings {
		if strings.Contains(w, "выкупить метод Непроглядного Тумана") && strings.Contains(w, "взят без") {
			found = true
		}
	}
	if !found {
		t.Errorf("ждал предупреждение про выкуп без урока, получил %v", sum.Warnings)
	}
}

// TestSkeletonDropsBrokenRequires — битая ссылка в requires не должна
// навсегда запирать накопитель или фолиант.
func TestSkeletonDropsBrokenRequires(t *testing.T) {
	raw := []byte(`{"name":"тест","balance":10,"shop":[{"key":"item:a","kind":"item","section":"other","title":"а","requires":["item:нет"]}],
		"accumulators":[{"name":"накопитель","requires":["item:нет","item:a"]}],
		"folios":[{"name":"фолиант","requires":["item:нет"]}]}`)
	sk, err := plan.ParseSkeleton(raw)
	if err != nil {
		t.Fatalf("каркас не разобрался: %v", err)
	}
	if len(sk.Shop[0].Requires) != 0 {
		t.Errorf("битая ссылка осталась у покупки: %v", sk.Shop[0].Requires)
	}
	if len(sk.Accumulators[0].Requires) != 1 || sk.Accumulators[0].Requires[0] != "item:a" {
		t.Errorf("у накопителя должны остаться только живые ссылки: %v", sk.Accumulators[0].Requires)
	}
	if len(sk.Folios[0].Requires) != 0 {
		t.Errorf("битая ссылка осталась у фолианта: %v", sk.Folios[0].Requires)
	}
}

// TestFolioRightsSeparate — право взять фолиант и право взять фолиант с доступом
// на стадии Накопления Ци — две разные покупки (5 и 10 XP), их нельзя схлопнуть.
func TestFolioRightsSeparate(t *testing.T) {
	sk := presetSkeleton(t)
	find := func(key string) *plan.ShopItem {
		for i := range sk.Shop {
			if sk.Shop[i].Key == key {
				return &sk.Shop[i]
			}
		}
		return nil
	}
	plain := find("folio:vzyat_1_foliant")
	nakoplenie := find("folio:vzyat_1_foliant_nakoplenie")
	if plain == nil || nakoplenie == nil {
		t.Fatalf("в каркасе должны быть обе опции библиотеки: обычная=%v, на Накоплении Ци=%v", plain != nil, nakoplenie != nil)
	}
	if plain.Cost != 5 || !plain.Fixed || plain.Repeat != 3 {
		t.Errorf("«взять 1 фолиант»: %v XP, fixed=%v, repeat=%d — ждал 5 XP и до трёх раз",
			plain.Cost, plain.Fixed, plain.Repeat)
	}
	if nakoplenie.Cost != 10 || !nakoplenie.Fixed || nakoplenie.Repeat != 3 {
		t.Errorf("«взять 1 фолиант на Накоплении Ци»: %v XP, fixed=%v, repeat=%d — ждал 10 XP и до трёх раз",
			nakoplenie.Cost, nakoplenie.Fixed, nakoplenie.Repeat)
	}
	// В плане они считаются как две отдельные строки.
	p := sk.NewPlan("u", "два права", "")
	p.Picks = append(p.Picks, plan.Pick{Key: "folio:vzyat_1_foliant"}, plan.Pick{Key: "folio:vzyat_1_foliant_nakoplenie"})
	sum := plan.Compute(p)
	got := 0.0
	for _, s := range sum.Sections {
		if s.Key == plan.SecFolio {
			got = s.Sum
		}
	}
	if got != 15 {
		t.Errorf("два права стоят 15 XP, посчиталось %v", got)
	}
}

// TestDeferredSlots — слоты «невыученных» приходят из каркаса: в этом
// посмертии их два (второй дал урок «невыученные уроки всё ещё не выучены»,
// выученный в прошлой жизни), урок с тегом defer_slot добавляет ещё один.
func TestDeferredSlots(t *testing.T) {
	sk := presetSkeleton(t)
	if sk.DeferredSlots != 2 {
		t.Errorf("в каркасе жизни 8 два слота невыученных, получил %d", sk.DeferredSlots)
	}
	if got := plan.Compute(sk.NewPlan("u", "два слота", "")).DeferredLimit; got != 2 {
		t.Errorf("план из каркаса: ждал 2 слота, получил %d", got)
	}
	// План без каркаса: слот по умолчанию один.
	if got := plan.Compute(plan.EmptyPlan("u", "пустой", "", 100)).DeferredLimit; got != 1 {
		t.Errorf("без каркаса слот один, получил %d", got)
	}

	raw := []byte(`{"name":"тест","balance":100,"shop":[
		{"key":"lesson:a","kind":"lesson","section":"lessons","title":"а","tags":["defer_slot"]}]}`)
	sk2, err := plan.ParseSkeleton(raw)
	if err != nil {
		t.Fatalf("каркас: %v", err)
	}
	p := sk2.NewPlan("u", "слот из урока", "")
	p.Picks = append(p.Picks, plan.Pick{Key: "lesson:a"})
	if got := plan.Compute(p).DeferredLimit; got != 2 {
		t.Errorf("взятый урок с тегом defer_slot даёт второй слот, получил %d", got)
	}
}
