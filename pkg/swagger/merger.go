package swagger

import (
	"encoding/json"
	"os"
)

type Merger struct {
	swagger map[string]any
}

func NewMerger() *Merger {
	return &Merger{
		swagger: map[string]any{},
	}
}

func (m *Merger) MarshalJSON() ([]byte, error) {
	return json.Marshal(m.swagger)
}

func (m *Merger) Add(b []byte) error {
	var f map[string]any
	if err := json.Unmarshal(b, &f); err != nil {
		return err
	}
	m.merge(f)
	return nil
}

func (m *Merger) AddFile(path string) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return m.Add(b)
}

func (m *Merger) merge(f map[string]any) {
	for k, v := range f {
		if i, ok := v.(map[string]any); ok {
			for sk, sv := range i {
				if _, ok := m.swagger[k]; !ok {
					m.swagger[k] = map[string]any{}
				}

				m.swagger[k].(map[string]any)[sk] = sv
			}
		} else {
			m.swagger[k] = v
		}
	}
}
