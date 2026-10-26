package plan

// Короткие названия для выгрузки. В плане, который читает Ли Лун, длинные
// формулировки из текста посмертия не нужны: «1. Мелисандра: 5» вместо
// «1. поцелуй Мелисандры: 5». Каркас может задать своё `short` у позиции,
// шкалы, накопителя или фолианта — тогда оно важнее карты; карта нужна для
// планов, сохранённых раньше, чем название появилось в каркасе.
var shortNames = map[string]string{
	// Уроки.
	"lesson:ping_sistem":                   "пинг систем",
	"lesson:uchene_svet":                   "ученье",
	"lesson:diletantka_shirokogo_profilya": "дилетантка",
	"lesson:prirozhdennyy_alhimik":         "алхимик",
	"lesson:sama_sebe_bezoar":              "безоар",
	"lesson:da_sdohni_zhe":                 "сдохни",
	"lesson:professionalnaya_deva":         "Дева",
	"lesson:nenavizhu_yady":                "без ядов",
	"lesson:prodlenie_bitvy":               "продление",
	"lesson:posledniy_boy":                 "последний бой",
	"lesson:poceluy_melisandry":            "Мелисандра",
	"lesson:povyshenie_chistoty_korney":    "+корни",
	"lesson:metod_ognennogo_vodopada":      "Водопад",
	"lesson:metod_neproglyadnogo_tumana":   "Туман",
	"lesson:metod_tonkogo_pogloscheniya":   "Инь",
	"lesson:metod_yadovityh_miazmov":       "Миазмы",
	"lesson:vodnoe_teloslozhenie":          "водное тело",
	"lesson:rodoslovnaya_raduzhnoy_foreli": "форель",
	"lesson:maksimum_zaradov_gorozhan":     "+макс горожан",
	"lesson:skorost_zaradki_gorozhan":      "+скорость горожан",
	"lesson:metod_ognennogo_dozhdya":       "Дождь",
	"lesson:nevyuchennye_uroki":            "невыученные",

	// Заплатки.
	"patch:otkluchenie_trenirovok": "без тренировок",
	"patch:bliznecy":               "близнецы",
	"patch:uslovnyy_otstoy":        "условный отстой",

	// Покупки.
	"item:pererozhdenie_dzhona":  "Джон Сноу",
	"item:vybor_pola":            "пол",
	"item:trinadcatyy_meridian":  "13-й меридиан",
	"item:vospominaniya_o_zemle": "Земля",
	"item:effektivnost_barera":   "барьер +1%",
	"item:effektivnost_rezervov": "резервы +1%",
	"item:effektivnost_tehnik":   "техники +1%",
	"item:skidka_na_tele":        "скидка на деактивацию",

	// Накопители: покупка та же позиция, что и активация.
	"item:hardkor":                "хардкор",
	"item:ravnovesie_zla_i_dobra": "равновесие",

	// Тренировки и навыки.
	"item:bazovoe_vyzhivanie": "выживание",
	"item:uchenica_kopiya":    "копьё",
	"item:diletantka_schita":  "щит",
	"item:uchenica_alhimika":  "алхимия",
	"item:lestnica_v_nebo":    "лестница",
	"item:ponaslyshke_mech":   "меч",

	// Библиотека.
	"folio:vzyat_1_foliant":            "1 фолиант",
	"folio:vzyat_1_foliant_nakoplenie": "фолиант на Накоплении Ци",
	"folio:vzyat_posle_okraski":        "фолиант после окраски",
	"folio:vzyat_posle_pola":           "фолиант после пола",
	"folio:vykupit_tuman":              "выкуп Тумана",
	"folio:vykupit_miazmy":             "выкуп Миазмов",
	"folio:vykupit_in":                 "выкуп Инь",
	"folio:vykupit_vodopad":            "выкуп Водопада",
	"folio:vykupit_dozhd":              "выкуп Дождя",

	// Накопители в плане (ключи каркаса).
	"acc:gorozhane":      "горожане",
	"acc:okrestnosti_mu": "Му",
	"acc:velikiy_uspeh":  "великий успех",
	"acc:hardkor":        "хардкор",
	"acc:ravnovesie":     "равновесие",

	// Фолианты в библиотеке (ключи каркаса).
	"folio:metod_neproglyadnogo_tumana": "Туман",
	"folio:metod_yadovityh_miazmov":     "Миазмы",
	"folio:metod_tonkogo_pogloscheniya": "Инь",
	"folio:metod_ognennogo_vodopada":    "Водопад",
	"folio:metod_ognennogo_dozhdya":     "Дождь",

	// Шкалы.
	"talent.die":          "кость",
	"talent.dice_count":   "костей",
	"talent.flat":         "бонус",
	"roots.max":           "корни макс",
	"roots.min":           "корни мин",
	"chance.billion":      "1/миллиард, ступень",
	"chance.million":      "1/миллион, ступень",
	"chance.million_home": "1/миллион дома",
}

// applyShorts раздаёт короткие названия тем позициям, у которых их нет:
// план хранит свою копию магазина, поэтому старые планы получают карту
// при чтении, а не после «Обновить из готового».
func (p *Plan) applyShorts() {
	for i := range p.Shop {
		it := &p.Shop[i]
		if it.Short == "" {
			it.Short = shortNames[it.Key]
		}
	}
	for i := range p.Ladders {
		l := &p.Ladders[i]
		if l.Short == "" {
			l.Short = shortNames[l.Key]
		}
	}
	for i := range p.Accumulators {
		a := &p.Accumulators[i]
		if a.Short == "" {
			a.Short = shortNames[a.Key]
		}
	}
	for i := range p.Folios {
		f := &p.Folios[i]
		if f.Short == "" {
			f.Short = shortNames[f.Key]
		}
	}
}

// ShortName — название позиции для выгрузки.
func (it *ShopItem) ShortName() string {
	if it.Short != "" {
		return it.Short
	}
	return displayTitle(it)
}

// ShortName — название накопителя для выгрузки.
func (a *Accumulator) ShortName() string {
	if a.Short != "" {
		return a.Short
	}
	return a.Name
}

// ShortName — название фолианта для выгрузки.
func (f *Folio) ShortName() string {
	if f.Short != "" {
		return f.Short
	}
	return f.Name
}

// ShortName — название шкалы для выгрузки.
func (l *LadderCfg) ShortName() string {
	if l.Short != "" {
		return l.Short
	}
	return l.Title
}
