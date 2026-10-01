package eve

import (
	"context"
	"github.com/jackc/pgx/v5/pgxpool"
	"glorynavy.local/seat/internal/modules/eve/internal/store"
	"glorynavy.local/seat/internal/platform/locale"
	"strings"
	"time"
)

// StaticDataService is the reusable module boundary for local type names and
// their lifecycle. Other modules inject this service, never its private store.
type StaticDataService struct {
	pool     *pgxpool.Pool
	source   sdeSource
	interval time.Duration
}
type StaticTypeName struct {
	ID                     int64
	Name, Language, Source string
}
type SDEUpdateStatus = store.SDEUpdateStatus

func staticName(ctx context.Context, id int64, chinese, english string) StaticTypeName {
	name, language := chinese, "zh"
	if locale.English(ctx) && english != "" || name == "" {
		name, language = english, "en"
	}
	return StaticTypeName{ID: id, Name: name, Language: language, Source: "sde"}
}

// SolarSystemNames reads the active local SDE in one batch, with no ESI calls.
func (s *StaticDataService) SolarSystemNames(ctx context.Context, ids []int64) (map[int64]StaticTypeName, error) {
	found := map[int64]StaticTypeName{}
	if len(ids) == 0 {
		return found, nil
	}
	rows, err := store.ReadSDESystemNames(ctx, s.pool, ids)
	if err != nil {
		return nil, err
	}
	for _, n := range rows {
		found[n.ID] = staticName(ctx, n.ID, n.Chinese, n.English)
	}
	return found, nil
}

func NewStaticData(pool *pgxpool.Pool, dir string, interval time.Duration) *StaticDataService {
	if interval <= 0 {
		interval = 6 * time.Hour
	}
	return &StaticDataService{pool: pool, source: newSDESource(dir), interval: interval}
}
func (s *StaticDataService) Status(ctx context.Context) (SDEUpdateStatus, error) {
	return store.SDEStatus(ctx, s.pool)
}
func (s *StaticDataService) Resume(ctx context.Context) error {
	return store.ResumeSDEUpdates(ctx, s.pool)
}

func (s *StaticDataService) TypeNames(ctx context.Context, ids []int64) (map[int64]StaticTypeName, error) {
	found := map[int64]StaticTypeName{}
	if len(ids) == 0 {
		return found, nil
	}
	rows, err := store.ReadSDETypeNames(ctx, s.pool, ids)
	if err != nil {
		return nil, err
	}
	for _, n := range rows {
		found[n.ID] = staticName(ctx, n.ID, n.Chinese, n.English)
	}
	missing := []int64{}
	seen := map[int64]bool{}
	for _, id := range ids {
		if _, ok := found[id]; !ok && !seen[id] && id >= 0 {
			missing = append(missing, id)
			seen[id] = true
		}
	}
	if len(missing) > 0 {
		languages := []string{"en", "zh"}
		if locale.English(ctx) {
			languages = []string{"zh", "en"}
		}
		for _, language := range languages {
			cached, e := store.New(s.pool).ReadEntityNames(ctx, store.ReadEntityNamesParams{Ids: missing, Language: language})
			if e != nil {
				return nil, e
			}
			for _, n := range cached {
				found[n.EntityID] = StaticTypeName{ID: n.EntityID, Name: n.Name, Language: language, Source: "esi_cache"}
			}
		}
	}
	return found, nil
}

// UpdateLatest is an operator-requested import. It avoids fetching the archive
// when the active build is already current; it does not remove an operator pin.
func (s *StaticDataService) UpdateLatest(ctx context.Context) (SDENamesRelease, error) {
	build, err := s.source.Latest(ctx)
	if err != nil {
		return SDENamesRelease{}, err
	}
	status, err := s.Status(ctx)
	if err != nil {
		return SDENamesRelease{}, err
	}
	if status.ActiveBuild > build || (status.ActiveBuild == build && status.MapperVersion >= 2) {
		return SDENamesRelease{ID: status.ActiveReleaseID, Build: status.ActiveBuild, Count: status.TypeCount, SystemCount: status.SystemCount, SHA256: status.ActiveSHA256, Reused: true}, nil
	}
	path, err := s.source.Download(ctx, build)
	if err != nil {
		return SDENamesRelease{}, err
	}
	return ImportSDENames(ctx, s.pool, path, build)
}

type SDENamesRelease = store.SDENamesRelease

func ImportSDENames(ctx context.Context, pool *pgxpool.Pool, path string, build int64) (SDENamesRelease, error) {
	return store.ImportSDENames(ctx, pool, path, build)
}
func ActivateSDENames(ctx context.Context, pool *pgxpool.Pool, id int64) error {
	return store.ActivateSDENames(ctx, pool, id)
}

func (s *StaticDataService) SearchTypes(ctx context.Context, term string) ([]StaticTypeName, error) {
	term = strings.TrimSpace(term)
	out := []StaticTypeName{}
	if len([]rune(term)) < 2 || len([]rune(term)) > 80 {
		return out, nil
	}
	rows, err := store.New(s.pool).SearchStaticTypes(ctx, term)
	if err != nil {
		return nil, err
	}
	for _, r := range rows {
		out = append(out, staticName(ctx, r.TypeID, r.NameZh, r.NameEn))
	}
	return out, nil
}
