package eve

import (
	"archive/zip"
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"sync"
)

// LPReference is derived from the currently active, verified official SDE ZIP.
// It has no separate database version or publication lifecycle.
type LPName struct {
	ID      int64  `json:"id"`
	Chinese string `json:"zh"`
	English string `json:"en"`
}
type LPCorporation struct {
	LPName
	FactionID int64 `json:"faction_id"`
}
type LPMaterial struct {
	TypeID   int64 `json:"type_id"`
	Quantity int64 `json:"quantity"`
}
type LPBlueprint struct {
	Product   LPMaterial   `json:"product"`
	Materials []LPMaterial `json:"materials"`
}
type LPReference struct {
	Build        int64                 `json:"build"`
	Corporations []LPCorporation       `json:"corporations"`
	Factions     []LPName              `json:"factions"`
	Blueprints   map[int64]LPBlueprint `json:"-"`
}
type LPDataService struct {
	Names *StaticDataService
	mu    sync.Mutex
	sha   string
	data  *LPReference
}

func (s *LPDataService) Reference(ctx context.Context) (*LPReference, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	status, err := s.Names.Status(ctx)
	if err != nil {
		return nil, err
	}
	if status.ActiveBuild <= 0 {
		return nil, errors.New("active SDE unavailable")
	}
	if s.data != nil && s.data.Build == status.ActiveBuild && s.sha == status.ActiveSHA256 {
		return s.data, nil
	}
	source, ok := s.Names.source.(*officialSDESource)
	if !ok {
		return nil, errors.New("official SDE source unavailable")
	}
	path := filepath.Join(source.dir, fmt.Sprintf("eve-online-static-data-%d-jsonl.zip", status.ActiveBuild))
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	h := sha256.New()
	_, err = io.Copy(h, io.LimitReader(f, (512<<20)+1))
	f.Close()
	if err != nil || hex.EncodeToString(h.Sum(nil)) != status.ActiveSHA256 {
		return nil, errors.New("SDE checksum mismatch")
	}
	data, err := readLPReference(path, status.ActiveBuild)
	if err != nil {
		return nil, err
	}
	// Recheck the active release after IO, including explicit version pins.
	current, err := s.Names.Status(ctx)
	if err != nil {
		return nil, err
	}
	if current.ActiveReleaseID != status.ActiveReleaseID {
		return nil, errors.New("SDE version changed")
	}
	s.data, s.sha = data, status.ActiveSHA256
	return data, nil
}

func readLPReference(path string, build int64) (*LPReference, error) {
	z, err := zip.OpenReader(path)
	if err != nil {
		return nil, err
	}
	defer z.Close()
	out := &LPReference{Build: build, Corporations: []LPCorporation{}, Factions: []LPName{}, Blueprints: map[int64]LPBlueprint{}}
	files := map[string]*zip.File{}
	for _, f := range z.File {
		if f.Name == "npcCorporations.jsonl" || f.Name == "factions.jsonl" || f.Name == "blueprints.jsonl" {
			if files[f.Name] != nil || f.UncompressedSize64 > 32<<20 {
				return nil, errors.New("invalid LP reference file")
			}
			files[f.Name] = f
		}
	}
	for _, name := range []string{"npcCorporations.jsonl", "factions.jsonl", "blueprints.jsonl"} {
		f := files[name]
		if f == nil {
			return nil, errors.New("LP reference file missing")
		}
		r, e := f.Open()
		if e != nil {
			return nil, e
		}
		scanner := bufio.NewScanner(io.LimitReader(r, (32<<20)+1))
		scanner.Buffer(make([]byte, 4096), 1<<20)
		seen := map[int64]bool{}
		for scanner.Scan() {
			var row struct {
				ID         int64             `json:"_key"`
				Name       map[string]string `json:"name"`
				Deleted    bool              `json:"deleted"`
				FactionID  int64             `json:"factionID"`
				Activities struct {
					Manufacturing struct {
						Products []struct {
							TypeID   int64 `json:"typeID"`
							Quantity int64 `json:"quantity"`
						} `json:"products"`
						Materials []struct {
							TypeID   int64 `json:"typeID"`
							Quantity int64 `json:"quantity"`
						} `json:"materials"`
					} `json:"manufacturing"`
				} `json:"activities"`
			}
			if json.Unmarshal(scanner.Bytes(), &row) != nil || row.ID <= 0 || seen[row.ID] || len(seen) >= 20000 {
				r.Close()
				return nil, errors.New("invalid LP reference row")
			}
			seen[row.ID] = true
			if name == "blueprints.jsonl" {
				b := LPBlueprint{Materials: []LPMaterial{}}
				m := row.Activities.Manufacturing
				if len(m.Products) == 1 {
					b.Product = LPMaterial{m.Products[0].TypeID, m.Products[0].Quantity}
					for _, item := range m.Materials {
						if item.TypeID <= 0 || item.Quantity <= 0 {
							r.Close()
							return nil, errors.New("invalid blueprint material")
						}
						b.Materials = append(b.Materials, LPMaterial{item.TypeID, item.Quantity})
					}
				}
				out.Blueprints[row.ID] = b
			} else if !row.Deleted {
				n := LPName{row.ID, row.Name["zh"], row.Name["en"]}
				if n.Chinese == "" && n.English == "" {
					r.Close()
					return nil, errors.New("LP name missing")
				}
				if name == "factions.jsonl" {
					out.Factions = append(out.Factions, n)
				} else {
					out.Corporations = append(out.Corporations, LPCorporation{n, row.FactionID})
				}
			}
		}
		err = scanner.Err()
		r.Close()
		if err != nil {
			return nil, err
		}
	}
	if len(out.Corporations) == 0 || len(out.Factions) == 0 || len(out.Blueprints) == 0 {
		return nil, errors.New("empty LP reference")
	}
	sort.Slice(out.Corporations, func(i, j int) bool { return out.Corporations[i].ID < out.Corporations[j].ID })
	return out, nil
}
