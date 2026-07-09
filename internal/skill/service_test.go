package skill

import (
	"bytes"
	"context"
	"errors"
	"io"
	"sort"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/skillhub/skillhub/internal/apperr"
	"github.com/skillhub/skillhub/internal/audit"
	"github.com/skillhub/skillhub/internal/storage"
	"go.uber.org/zap"
)

type memStore struct{ objs map[string][]byte }

func newMemStore() *memStore { return &memStore{objs: map[string][]byte{}} }

func (m *memStore) Put(ctx context.Context, key string, r io.Reader, size int64, ct string) (string, error) {
	b, _ := io.ReadAll(r)
	m.objs[key] = b
	return key, nil
}
func (m *memStore) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	b, ok := m.objs[key]
	if !ok {
		return nil, errors.New("not found")
	}
	return io.NopCloser(bytes.NewReader(b)), nil
}
func (m *memStore) Delete(ctx context.Context, key string) error { delete(m.objs, key); return nil }
func (m *memStore) Stat(ctx context.Context, key string) (storage.ObjectInfo, error) {
	return storage.ObjectInfo{}, nil
}

type mockSkillRepo struct {
	skills   map[uuid.UUID]*Skill
	versions map[uuid.UUID][]SkillVersion
	stars    map[[2]uuid.UUID]bool // (userID, skillID)
}

func newMockSkillRepo() *mockSkillRepo {
	return &mockSkillRepo{skills: map[uuid.UUID]*Skill{}, versions: map[uuid.UUID][]SkillVersion{}, stars: map[[2]uuid.UUID]bool{}}
}

func (m *mockSkillRepo) CreateSkill(ctx context.Context, s *Skill) error {
	for _, e := range m.skills {
		if e.TeamID == s.TeamID && e.Name == s.Name {
			return apperr.New("conflict", "skill", "skill already exists")
		}
	}
	s.ID = uuid.New()
	m.skills[s.ID] = s
	return nil
}
func (m *mockSkillRepo) GetSkill(ctx context.Context, teamID uuid.UUID, name string) (*Skill, error) {
	for _, e := range m.skills {
		if e.TeamID == teamID && e.Name == name {
			return e, nil
		}
	}
	return nil, apperr.New("not_found", "skill", "skill not found")
}
func (m *mockSkillRepo) GetSkillByID(ctx context.Context, skillID uuid.UUID) (*Skill, error) {
	if s, ok := m.skills[skillID]; ok {
		return s, nil
	}
	return nil, apperr.New("not_found", "skill", "skill not found")
}
func (m *mockSkillRepo) ListSkillsByTeam(ctx context.Context, teamID uuid.UUID) ([]Skill, error) {
	var out []Skill
	for _, e := range m.skills {
		if e.TeamID == teamID {
			out = append(out, *e)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}
func (m *mockSkillRepo) CreateVersion(ctx context.Context, v *SkillVersion) error {
	for _, e := range m.versions[v.SkillID] {
		if e.Version == v.Version {
			return apperr.New("conflict", "skill", "version already exists")
		}
	}
	v.ID = uuid.New()
	m.versions[v.SkillID] = append(m.versions[v.SkillID], *v)
	return nil
}
func (m *mockSkillRepo) GetVersion(ctx context.Context, skillID uuid.UUID, version string) (*SkillVersion, error) {
	for _, e := range m.versions[skillID] {
		if e.Version == version {
			v := e
			return &v, nil
		}
	}
	return nil, apperr.New("not_found", "skill", "version not found")
}
func (m *mockSkillRepo) ListVersions(ctx context.Context, skillID uuid.UUID) ([]SkillVersion, error) {
	out := make([]SkillVersion, len(m.versions[skillID]))
	copy(out, m.versions[skillID])
	return out, nil
}
func (m *mockSkillRepo) Search(ctx context.Context, teamIDs []uuid.UUID, q string, limit, offset int) ([]SearchRow, error) {
	visible := map[uuid.UUID]bool{}
	for _, tid := range teamIDs {
		visible[tid] = true
	}
	var out []SearchRow
	for _, sk := range m.skills {
		if !visible[sk.TeamID] {
			continue
		}
		if q != "" && !strings.Contains(sk.Name, q) {
			continue
		}
		out = append(out, SearchRow{Skill: *sk, TeamSlug: "mock"})
	}
	if offset >= len(out) {
		return nil, nil
	}
	out = out[offset:]
	if limit < len(out) {
		out = out[:limit]
	}
	return out, nil
}

func (m *mockSkillRepo) Star(ctx context.Context, userID, skillID uuid.UUID) error {
	m.stars[[2]uuid.UUID{userID, skillID}] = true
	return nil
}
func (m *mockSkillRepo) Unstar(ctx context.Context, userID, skillID uuid.UUID) error {
	delete(m.stars, [2]uuid.UUID{userID, skillID})
	return nil
}
func (m *mockSkillRepo) IsStarred(ctx context.Context, userID, skillID uuid.UUID) (bool, error) {
	return m.stars[[2]uuid.UUID{userID, skillID}], nil
}
func (m *mockSkillRepo) CountStars(ctx context.Context, skillID uuid.UUID) (int64, error) {
	var n int64
	for k := range m.stars {
		if k[1] == skillID {
			n++
		}
	}
	return n, nil
}
func (m *mockSkillRepo) ListStarredSkills(ctx context.Context, userID uuid.UUID, limit, offset int) ([]SearchRow, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	if offset < 0 {
		offset = 0
	}
	var out []SearchRow
	for k := range m.stars {
		if k[0] != userID {
			continue
		}
		if sk, ok := m.skills[k[1]]; ok {
			out = append(out, SearchRow{Skill: *sk, TeamSlug: "mock"})
		}
	}
	if offset >= len(out) {
		return nil, nil
	}
	out = out[offset:]
	if limit < len(out) {
		out = out[:limit]
	}
	return out, nil
}

func newSkillSvc() (*Service, *mockSkillRepo, *memStore) {
	r := newMockSkillRepo()
	st := newMemStore()
	return NewService(r, st, audit.NewLogger(nil, zap.NewNop())), r, st
}

func TestPublish_NewSkillAndVersion(t *testing.T) {
	s, r, st := newSkillSvc()
	ctx := context.Background()
	tid := uuid.New()
	pub := uuid.New()
	body := bytes.NewReader([]byte("tarball"))
	sv, err := s.Publish(ctx, tid, "my-skill", "1.0.0", body, 7, ContentTypeTarball, pub)
	if err != nil {
		t.Fatal(err)
	}
	if sv.Sha256 == "" {
		t.Fatal("sha empty")
	}
	if len(r.versions[sv.SkillID]) != 1 {
		t.Fatal("version not stored")
	}
	if _, ok := st.objs[sv.StorageKey]; !ok {
		t.Fatal("object not stored")
	}
}

func TestPublish_DuplicateVersion_Conflict(t *testing.T) {
	s, _, _ := newSkillSvc()
	ctx := context.Background()
	tid := uuid.New()
	pub := uuid.New()
	_, _ = s.Publish(ctx, tid, "my-skill", "1.0.0", bytes.NewReader([]byte("payload")), 7, ContentTypeTarball, pub)
	_, err := s.Publish(ctx, tid, "my-skill", "1.0.0", bytes.NewReader([]byte("payload")), 7, ContentTypeTarball, pub)
	if err == nil {
		t.Fatal("expected conflict")
	}
	e, ok := err.(*apperr.Error)
	if !ok || e.Code != "conflict" {
		t.Fatalf("expected conflict, got %v", err)
	}
	// 原版本对象必须完好：仍可下载
	rc, _, err := s.OpenVersion(ctx, tid, "my-skill", "1.0.0")
	if err != nil {
		t.Fatalf("download after duplicate: %v", err)
	}
	defer rc.Close()
	b, _ := io.ReadAll(rc)
	if string(b) != "payload" {
		t.Fatalf("corrupted payload: %q", b)
	}
}

func TestPublish_InvalidNameOrVersion(t *testing.T) {
	s, _, _ := newSkillSvc()
	ctx := context.Background()
	tid := uuid.New()
	pub := uuid.New()
	if _, err := s.Publish(ctx, tid, "BadName", "1.0.0", bytes.NewReader([]byte("a")), 1, ContentTypeTarball, pub); err == nil {
		t.Fatal("expected invalid name")
	}
	if _, err := s.Publish(ctx, tid, "ok", "not-semver", bytes.NewReader([]byte("a")), 1, ContentTypeTarball, pub); err == nil {
		t.Fatal("expected invalid version")
	}
}

func TestOpenVersion(t *testing.T) {
	s, _, _ := newSkillSvc()
	ctx := context.Background()
	tid := uuid.New()
	pub := uuid.New()
	_, _ = s.Publish(ctx, tid, "my-skill", "1.0.0", bytes.NewReader([]byte("payload")), 7, ContentTypeTarball, pub)
	rc, sv, err := s.OpenVersion(ctx, tid, "my-skill", "1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	defer rc.Close()
	b, _ := io.ReadAll(rc)
	if string(b) != "payload" {
		t.Fatalf("got %q", b)
	}
	if sv.Size != 7 {
		t.Fatalf("size=%d", sv.Size)
	}
}

func TestService_Search_LatestVersion(t *testing.T) {
	s, _, _ := newSkillSvc()
	ctx := context.Background()
	tid := uuid.New()
	pub := uuid.New()
	_, _ = s.Publish(ctx, tid, "go-lint", "1.0.0", bytes.NewReader([]byte("a")), 1, ContentTypeTarball, pub)
	_, _ = s.Publish(ctx, tid, "go-lint", "1.2.0", bytes.NewReader([]byte("b")), 1, ContentTypeTarball, pub)
	_, _ = s.Publish(ctx, tid, "go-format", "0.1.0", bytes.NewReader([]byte("c")), 1, ContentTypeTarball, pub)

	res, err := s.Search(ctx, []uuid.UUID{tid}, "lint", 1, 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(res) != 1 || res[0].Name != "go-lint" {
		t.Fatalf("res=%+v", res)
	}
	if res[0].LatestVersion == nil || res[0].LatestVersion.Version != "1.2.0" {
		t.Fatalf("latest=%+v", res[0].LatestVersion)
	}
}

func TestService_Search_PaginationClamp(t *testing.T) {
	s, _, _ := newSkillSvc()
	ctx := context.Background()
	tid := uuid.New()
	// page<1 → 1; pageSize>100 → 100; 不报错
	if _, err := s.Search(ctx, []uuid.UUID{tid}, "", 0, 500); err != nil {
		t.Fatal(err)
	}
	// q 太长 → validation_failed
	longQ := strings.Repeat("a", 257)
	if _, err := s.Search(ctx, []uuid.UUID{tid}, longQ, 1, 20); err == nil {
		t.Fatal("expected query too long")
	}
}

func TestPromoteToGlobal(t *testing.T) {
	s, _, st := newSkillSvc()
	ctx := context.Background()
	tid := uuid.New()
	pub := uuid.New()
	src, _ := s.Publish(ctx, tid, "go-lint", "1.0.0", bytes.NewReader([]byte("payload")), 7, ContentTypeTarball, pub)
	globalTeam := uuid.New()
	admin := uuid.New()

	gv, err := s.PromoteToGlobal(ctx, src.SkillID, "1.0.0", globalTeam, "go-lint", admin)
	if err != nil {
		t.Fatal(err)
	}
	if gv.SkillID == src.SkillID {
		t.Fatal("global version should belong to a different skill")
	}
	if gv.Sha256 != src.Sha256 {
		t.Fatal("sha mismatch")
	}
	if _, ok := st.objs[gv.StorageKey]; !ok {
		t.Fatal("global object not stored")
	}
	if gv.StorageKey == src.StorageKey {
		t.Fatal("should be a copy, not same key")
	}
}

func TestPromoteToGlobal_VersionConflict(t *testing.T) {
	s, _, _ := newSkillSvc()
	ctx := context.Background()
	tid := uuid.New()
	pub := uuid.New()
	src, _ := s.Publish(ctx, tid, "go-lint", "1.0.0", bytes.NewReader([]byte("a")), 1, ContentTypeTarball, pub)
	globalTeam := uuid.New()
	admin := uuid.New()
	_, _ = s.PromoteToGlobal(ctx, src.SkillID, "1.0.0", globalTeam, "go-lint", admin)
	_, err := s.PromoteToGlobal(ctx, src.SkillID, "1.0.0", globalTeam, "go-lint", admin)
	if err == nil {
		t.Fatal("expected conflict")
	}
	e, ok := err.(*apperr.Error)
	if !ok || e.Code != "conflict" {
		t.Fatalf("expected conflict, got %v", err)
	}
}

func TestPromoteToGlobal_SourceVersionMissing(t *testing.T) {
	s, _, _ := newSkillSvc()
	ctx := context.Background()
	tid := uuid.New()
	pub := uuid.New()
	src, _ := s.Publish(ctx, tid, "go-lint", "1.0.0", bytes.NewReader([]byte("a")), 1, ContentTypeTarball, pub)
	globalTeam := uuid.New()
	admin := uuid.New()
	_, err := s.PromoteToGlobal(ctx, src.SkillID, "9.9.9", globalTeam, "go-lint", admin)
	if err == nil {
		t.Fatal("expected not found")
	}
}

func TestPromoteToGlobal_InvalidTargetName(t *testing.T) {
	s, _, _ := newSkillSvc()
	ctx := context.Background()
	tid := uuid.New()
	pub := uuid.New()
	src, _ := s.Publish(ctx, tid, "go-lint", "1.0.0", bytes.NewReader([]byte("a")), 1, ContentTypeTarball, pub)
	globalTeam := uuid.New()
	admin := uuid.New()
	_, err := s.PromoteToGlobal(ctx, src.SkillID, "1.0.0", globalTeam, "BadName", admin)
	if err == nil {
		t.Fatal("expected invalid name")
	}
}
