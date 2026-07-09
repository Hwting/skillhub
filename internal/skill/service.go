package skill

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"sort"

	"github.com/google/uuid"
	"github.com/skillhub/skillhub/internal/apperr"
	"github.com/skillhub/skillhub/internal/audit"
	"github.com/skillhub/skillhub/internal/storage"
)

type Service struct {
	repo  Repo
	store storage.Store
	audit *audit.Logger
}

func NewService(repo Repo, store storage.Store, audit *audit.Logger) *Service {
	return &Service{repo: repo, store: store, audit: audit}
}

// Repo exposes the underlying repository for read-only lookups in handlers.
func (s *Service) Repo() Repo { return s.repo }

// Publish stores a new immutable version of a skill. Creates the skill row
// on first publish. Returns conflict (409) if the version already exists;
// on that or any DB failure after the object was uploaded, the orphan
// storage object is deleted.
func (s *Service) Publish(ctx context.Context, teamID uuid.UUID, name, version string, r io.Reader, size int64, contentType string, publisherID uuid.UUID) (*SkillVersion, error) {
	if !IsValidName(name) {
		return nil, apperr.New("validation_failed", "skill", "invalid skill name")
	}
	if !IsValid(version) {
		return nil, apperr.New("validation_failed", "skill", "invalid version")
	}
	if size < 0 || size > MaxPackageSize {
		return nil, apperr.New("validation_failed", "skill", "package too large")
	}
	buf := &bytes.Buffer{}
	if _, err := io.Copy(buf, r); err != nil {
		return nil, apperr.New("validation_failed", "skill", "read body failed")
	}
	if int64(buf.Len()) > MaxPackageSize {
		return nil, apperr.New("validation_failed", "skill", "package too large")
	}
	sum := sha256.Sum256(buf.Bytes())
	sha := hex.EncodeToString(sum[:])

	// 找或建 skill
	sk, err := s.repo.GetSkill(ctx, teamID, name)
	if err != nil {
		if !isNotFound(err) {
			return nil, err
		}
		sk = &Skill{TeamID: teamID, Name: name}
		if err := s.repo.CreateSkill(ctx, sk); err != nil {
			return nil, err
		}
		_ = s.audit.Log(ctx, audit.Entry{ActorUserID: &publisherID, Action: audit.Action("skill_created"), TargetType: "skill", TargetID: sk.ID.String(), Metadata: map[string]any{"name": name, "team_id": teamID.String()}})
	}
	key := fmt.Sprintf("skills/%s/%s/%s.tar.gz", sk.ID.String(), version, sha)
	if _, err := s.store.Put(ctx, key, bytes.NewReader(buf.Bytes()), int64(buf.Len()), contentType); err != nil {
		return nil, fmt.Errorf("store put: %w", err)
	}
	sv := &SkillVersion{
		SkillID:         sk.ID,
		Version:         version,
		StorageKey:      key,
		Size:            int64(buf.Len()),
		Sha256:          sha,
		ContentType:     contentType,
		PublisherUserID: publisherID,
	}
	if err := s.repo.CreateVersion(ctx, sv); err != nil {
		// 仅在非 conflict 时清理：conflict 意味着该 version 已存在，其对象
		// 可能与我们刚写入的 key 相同（同内容），删除会破坏既有版本。
		if !isConflict(err) {
			_ = s.store.Delete(ctx, key)
		}
		return nil, err
	}
	_ = s.audit.Log(ctx, audit.Entry{ActorUserID: &publisherID, Action: audit.Action("skill_version_published"), TargetType: "skill_version", TargetID: sv.ID.String(), Metadata: map[string]any{"skill_id": sk.ID.String(), "version": version, "sha256": sha}})
	return sv, nil
}

func (s *Service) GetSkillWithVersions(ctx context.Context, teamID uuid.UUID, name string) (*Skill, []SkillVersion, error) {
	sk, err := s.repo.GetSkill(ctx, teamID, name)
	if err != nil {
		return nil, nil, err
	}
	vs, err := s.repo.ListVersions(ctx, sk.ID)
	if err != nil {
		return nil, nil, err
	}
	sort.Slice(vs, func(i, j int) bool { return Compare(vs[i].Version, vs[j].Version) > 0 })
	return sk, vs, nil
}

func (s *Service) ListSkillsByTeam(ctx context.Context, teamID uuid.UUID) ([]Skill, error) {
	return s.repo.ListSkillsByTeam(ctx, teamID)
}

// SkillDetail is a skill with its versions, star count, and whether the
// requesting user has starred it — the payload for skill detail views.
type SkillDetail struct {
	Skill
	Versions  []SkillVersion
	StarCount int64
	IsStarred bool
}

// GetSkillDetail returns a skill with its versions (sorted newest semver
// first), star count, and the viewer's starred state. CountStars/IsStarred
// errors are ignored (counts are non-critical; 0/false is a safe default).
func (s *Service) GetSkillDetail(ctx context.Context, teamID uuid.UUID, name string, viewerID uuid.UUID) (*SkillDetail, error) {
	sk, err := s.repo.GetSkill(ctx, teamID, name)
	if err != nil {
		return nil, err
	}
	vs, err := s.repo.ListVersions(ctx, sk.ID)
	if err != nil {
		return nil, err
	}
	sort.Slice(vs, func(i, j int) bool { return Compare(vs[i].Version, vs[j].Version) > 0 })
	d := &SkillDetail{Skill: *sk, Versions: vs}
	if count, err := s.repo.CountStars(ctx, sk.ID); err == nil {
		d.StarCount = count
	}
	if viewerID != uuid.Nil {
		if starred, err := s.repo.IsStarred(ctx, viewerID, sk.ID); err == nil {
			d.IsStarred = starred
		}
	}
	return d, nil
}

// Star records userID's star on the skill identified by (teamID, name).
func (s *Service) Star(ctx context.Context, userID, teamID uuid.UUID, name string) error {
	sk, err := s.repo.GetSkill(ctx, teamID, name)
	if err != nil {
		return err
	}
	return s.repo.Star(ctx, userID, sk.ID)
}

// Unstar removes userID's star on the skill identified by (teamID, name).
func (s *Service) Unstar(ctx context.Context, userID, teamID uuid.UUID, name string) error {
	sk, err := s.repo.GetSkill(ctx, teamID, name)
	if err != nil {
		return err
	}
	return s.repo.Unstar(ctx, userID, sk.ID)
}

// ListMyStars returns the skills the user has starred, newest star first, with
// the latest version attached. page is 1-based; pageSize defaults to 20 and is
// clamped to 100.
func (s *Service) ListMyStars(ctx context.Context, userID uuid.UUID, page, pageSize int) ([]SearchResult, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}
	offset := (page - 1) * pageSize
	rows, err := s.repo.ListStarredSkills(ctx, userID, pageSize, offset)
	if err != nil {
		return nil, err
	}
	out := make([]SearchResult, len(rows))
	for i, row := range rows {
		out[i] = SearchResult{Skill: row.Skill, TeamSlug: row.TeamSlug}
		vs, err := s.repo.ListVersions(ctx, row.ID)
		if err != nil {
			return nil, err
		}
		out[i].LatestVersion = latestVersion(vs)
	}
	return out, nil
}

// OpenVersion returns a stream over the stored tarball plus its metadata.
// Caller must close the stream.
func (s *Service) OpenVersion(ctx context.Context, teamID uuid.UUID, name, version string) (io.ReadCloser, *SkillVersion, error) {
	sk, err := s.repo.GetSkill(ctx, teamID, name)
	if err != nil {
		return nil, nil, err
	}
	sv, err := s.repo.GetVersion(ctx, sk.ID, version)
	if err != nil {
		return nil, nil, err
	}
	rc, err := s.store.Get(ctx, sv.StorageKey)
	if err != nil {
		return nil, nil, fmt.Errorf("store get: %w", err)
	}
	return rc, sv, nil
}

func isNotFound(err error) bool {
	e, ok := err.(*apperr.Error)
	return ok && e.Code == "not_found"
}

func isConflict(err error) bool {
	e, ok := err.(*apperr.Error)
	return ok && e.Code == "conflict"
}

// SearchResult is a skill plus its team slug and latest version (by semver),
// for display in search listings.
type SearchResult struct {
	Skill
	TeamSlug      string
	LatestVersion *SkillVersion // nil if the skill has no versions yet
}

// Search returns the skills visible to the given team set, optionally filtered
// by a name full-text query, with the latest version attached. page is 1-based;
// pageSize defaults to 20 and is clamped to 100.
func (s *Service) Search(ctx context.Context, teamIDs []uuid.UUID, q string, page, pageSize int) ([]SearchResult, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}
	if len(q) > 256 {
		return nil, apperr.New("validation_failed", "skill", "query too long")
	}
	offset := (page - 1) * pageSize
	rows, err := s.repo.Search(ctx, teamIDs, q, pageSize, offset)
	if err != nil {
		return nil, err
	}
	out := make([]SearchResult, len(rows))
	for i, row := range rows {
		out[i] = SearchResult{Skill: row.Skill, TeamSlug: row.TeamSlug}
		vs, err := s.repo.ListVersions(ctx, row.ID)
		if err != nil {
			return nil, err
		}
		out[i].LatestVersion = latestVersion(vs)
	}
	return out, nil
}

func latestVersion(vs []SkillVersion) *SkillVersion {
	if len(vs) == 0 {
		return nil
	}
	best := &vs[0]
	for i := 1; i < len(vs); i++ {
		if Compare(vs[i].Version, best.Version) > 0 {
			best = &vs[i]
		}
	}
	return best
}

// PromoteToGlobal copies a team skill's version into the global namespace under
// targetName. If a global skill with targetName already exists, the version is
// added to it (the version must not already exist there). The source storage
// object is copied to a global-owned key so the global version is self-contained.
func (s *Service) PromoteToGlobal(ctx context.Context, srcSkillID uuid.UUID, version string, globalTeamID uuid.UUID, targetName string, adminID uuid.UUID) (*SkillVersion, error) {
	if !IsValidName(targetName) {
		return nil, apperr.New("validation_failed", "skill", "invalid target name")
	}
	srcVer, err := s.repo.GetVersion(ctx, srcSkillID, version)
	if err != nil {
		return nil, err
	}

	// 找或建 global skill
	gSkill, err := s.repo.GetSkill(ctx, globalTeamID, targetName)
	if err != nil {
		if !isNotFound(err) {
			return nil, err
		}
		gSkill = &Skill{TeamID: globalTeamID, Name: targetName}
		if err := s.repo.CreateSkill(ctx, gSkill); err != nil {
			return nil, err
		}
	}
	// version 已存在 → conflict
	if existing, err := s.repo.GetVersion(ctx, gSkill.ID, version); err == nil && existing != nil {
		return nil, apperr.New("conflict", "skill", "version already exists in global")
	}

	// 复制对象
	rc, err := s.store.Get(ctx, srcVer.StorageKey)
	if err != nil {
		return nil, fmt.Errorf("store get source: %w", err)
	}
	defer rc.Close()
	buf := &bytes.Buffer{}
	if _, err := io.Copy(buf, rc); err != nil {
		return nil, apperr.New("validation_failed", "skill", "read source failed")
	}
	sum := sha256.Sum256(buf.Bytes())
	sha := hex.EncodeToString(sum[:])
	if sha != srcVer.Sha256 {
		return nil, apperr.New("db_error", "skill", "integrity check failed")
	}
	newKey := fmt.Sprintf("skills/%s/%s/%s.tar.gz", gSkill.ID.String(), version, sha)
	if _, err := s.store.Put(ctx, newKey, bytes.NewReader(buf.Bytes()), int64(buf.Len()), srcVer.ContentType); err != nil {
		return nil, fmt.Errorf("store put global: %w", err)
	}

	gv := &SkillVersion{
		SkillID:         gSkill.ID,
		Version:         version,
		StorageKey:      newKey,
		Size:            int64(buf.Len()),
		Sha256:          sha,
		ContentType:     srcVer.ContentType,
		PublisherUserID: adminID,
	}
	if err := s.repo.CreateVersion(ctx, gv); err != nil {
		if !isConflict(err) {
			_ = s.store.Delete(ctx, newKey)
		}
		return nil, err
	}
	_ = s.audit.Log(ctx, audit.Entry{ActorUserID: &adminID, Action: audit.Action("skill_promoted_to_global"), TargetType: "skill_version", TargetID: gv.ID.String(), Metadata: map[string]any{"src_skill_id": srcSkillID.String(), "global_skill_id": gSkill.ID.String(), "version": version, "target_name": targetName}})
	return gv, nil
}
