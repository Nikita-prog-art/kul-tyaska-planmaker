package web

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"sort"
	"strings"
	"sync"

	"kultyaska/internal/plan"
)

//go:embed presets/*.json
var presetFS embed.FS

// PresetInfo — краткое описание каркаса для списка.
type PresetInfo struct {
	Name        string   `json:"name"`
	File        string   `json:"file"`
	Balance     float64  `json:"balance"`
	Earned      float64  `json:"earned"`
	Lessons     int      `json:"lessons"`
	Items       int      `json:"items"`
	Note        string   `json:"note,omitempty"`
	Multipliers []string `json:"multipliers,omitempty"`
}

var (
	presetOnce   sync.Once
	presetCache  map[string]*plan.Skeleton
	presetInfoCh []PresetInfo
	presetErr    error
)

func loadPresets() {
	presetCache = map[string]*plan.Skeleton{}
	entries, err := fs.ReadDir(presetFS, "presets")
	if err != nil {
		presetErr = err
		return
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		raw, err := presetFS.ReadFile("presets/" + e.Name())
		if err != nil {
			presetErr = err
			continue
		}
		sk, err := plan.ParseSkeleton(raw)
		if err != nil {
			presetErr = fmt.Errorf("каркас %s: %w", e.Name(), err)
			continue
		}
		key := strings.TrimSuffix(e.Name(), ".json")
		presetCache[key] = sk
		presetCache[strings.ToLower(sk.Name)] = sk
		info := PresetInfo{
			Name: sk.Name, File: key, Balance: sk.Balance, Earned: sk.Earned,
			Note: sk.Notes, Multipliers: sk.Multipliers,
		}
		for _, it := range sk.Shop {
			if it.Kind == plan.KindLesson {
				info.Lessons++
			} else {
				info.Items++
			}
		}
		presetInfoCh = append(presetInfoCh, info)
	}
	sort.Slice(presetInfoCh, func(i, j int) bool { return presetInfoCh[i].Name < presetInfoCh[j].Name })
}

func (s *Server) presetList() []PresetInfo {
	presetOnce.Do(loadPresets)
	if presetErr != nil {
		return nil
	}
	return presetInfoCh
}

func (s *Server) preset(name string) (*plan.Skeleton, error) {
	presetOnce.Do(loadPresets)
	if sk, ok := presetCache[name]; ok {
		return sk, nil
	}
	if sk, ok := presetCache[strings.ToLower(name)]; ok {
		return sk, nil
	}
	return nil, fmt.Errorf("каркас %q не найден", name)
}

func (s *Server) handlePreset(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimSuffix(r.PathValue("name"), ".json")
	sk, err := s.preset(name)
	if err != nil {
		http.Error(w, "каркас не найден", http.StatusNotFound)
		return
	}
	raw, err := json.MarshalIndent(sk, "", "  ")
	if err != nil {
		http.Error(w, "не удалось сериализовать каркас", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", name+".json"))
	_, _ = w.Write(raw)
}
