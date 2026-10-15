package store_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"kultyaska/internal/plan"
	"kultyaska/internal/store"
)

// TestReadNormalizesLegacyPlan — регрессия: в базе могли остаться планы,
// записанные старым кодом, где пустые списки уезжали как null.
// При чтении они должны превращаться в пустые массивы, иначе конструктор
// в браузере падает на plan.picks.some(...).
func TestReadNormalizesLegacyPlan(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "db.json")
	legacy := `{
	  "version": 1,
	  "users": [{"id":"u1","nick":"archi","discord":"@archi","pass_hash":"x","is_admin":true,"created_at":"2026-01-01T00:00:00Z"}],
	  "sessions": {},
	  "plans": {
	    "p1": {
	      "id": "p1", "user_id": "u1", "title": "Старый план", "balance": 100,
	      "shop": null, "picks": null, "deferred": null, "not_taken": null,
	      "accumulators": null, "folios": null, "multipliers": null, "ladders": null,
	      "created_at": "2026-01-01T00:00:00Z", "updated_at": "2026-01-01T00:00:00Z"
	    }
	  }
	}`
	if err := os.WriteFile(path, []byte(legacy), 0o600); err != nil {
		t.Fatal(err)
	}

	st, err := store.Open(path)
	if err != nil {
		t.Fatalf("открыть базу: %v", err)
	}
	defer st.Close()

	p, err := st.Plan("p1")
	if err != nil {
		t.Fatalf("план: %v", err)
	}
	if p.Picks == nil || p.Deferred == nil || p.NotTaken == nil {
		t.Error("списки выбора должны быть пустыми массивами, а не nil")
	}
	if p.Shop == nil || p.Accumulators == nil || p.Folios == nil {
		t.Error("списки магазина должны быть пустыми массивами, а не nil")
	}
	if p.Ladders == nil || len(p.Ladders) == 0 {
		t.Error("шкалы должны подставиться по умолчанию")
	}

	list, err := st.PlansByUser("u1")
	if err != nil || len(list) != 1 {
		t.Fatalf("список планов: %v, %d", err, len(list))
	}
	if list[0].Picks == nil {
		t.Error("в списке планов покупки тоже должны быть массивом")
	}
}

// TestPutPlanNormalizes проверяет, что в базу null-списки не попадают.
func TestPutPlanNormalizes(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "db.json")
	st, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	p := plan.EmptyPlan("u1", "Пустой", "", 100)
	// Намеренно портим план как старый код: пустые списки — nil.
	p.Shop, p.Picks, p.Deferred, p.NotTaken = nil, nil, nil, nil
	p.Accumulators, p.Folios, p.Mults, p.Ladders = nil, nil, nil, nil
	if err := st.PutPlan(p); err != nil {
		t.Fatalf("сохранить план: %v", err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{`"picks": null`, `"shop": null`, `"deferred": null`, `"not_taken": null`} {
		if strings.Contains(string(raw), bad) {
			t.Errorf("в базе осталось %s:\n%s", bad, raw)
		}
	}
	stored, err := st.Plan(p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Picks == nil || stored.Shop == nil {
		t.Error("прочитанный план должен иметь пустые массивы")
	}
}
