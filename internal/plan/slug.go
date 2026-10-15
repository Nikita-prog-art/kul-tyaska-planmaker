package plan

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"
	"time"
)

// Slug строит стабильный ключ из названия (транслит + подчёркивания).
func Slug(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if t, ok := translit[r]; ok {
			b.WriteString(t)
			continue
		}
		b.WriteRune(r)
	}
	out := reSlug.ReplaceAllString(b.String(), "_")
	out = strings.Trim(out, "_")
	if len(out) > 48 {
		out = out[:48]
	}
	if out == "" {
		out = "item"
	}
	return out
}

var (
	reSlug = regexp.MustCompile(`[^a-z0-9]+`)

	translit = map[rune]string{
		'а': "a", 'б': "b", 'в': "v", 'г': "g", 'д': "d", 'е': "e", 'ё': "e", 'ж': "zh",
		'з': "z", 'и': "i", 'й': "y", 'к': "k", 'л': "l", 'м': "m", 'н': "n", 'о': "o",
		'п': "p", 'р': "r", 'с': "s", 'т': "t", 'у': "u", 'ф': "f", 'х': "h", 'ц': "c",
		'ч': "ch", 'ш': "sh", 'щ': "sch", 'ъ': "", 'ы': "y", 'ь': "", 'э': "e", 'ю': "yu",
		'я': "ya", '«': "", '»': "", '"': "", '\'': "", '№': "n", '–': "", '—': "",
	}
)

func genID() string {
	buf := make([]byte, 12)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(buf)
}
