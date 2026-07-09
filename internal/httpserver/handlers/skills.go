package handlers

import (
	"io"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/skillhub/skillhub/internal/apperr"
	"github.com/skillhub/skillhub/internal/auth"
	"github.com/skillhub/skillhub/internal/skill"
	"github.com/skillhub/skillhub/internal/team"
)

const timeRFC3339 = "2006-01-02T15:04:05Z07:00"

type SkillHandlers struct {
	svc     *skill.Service
	teamSvc *team.Service
}

func NewSkillHandlers(svc *skill.Service, teamSvc *team.Service) *SkillHandlers {
	return &SkillHandlers{svc: svc, teamSvc: teamSvc}
}

type skillResp struct {
	ID     string `json:"id"`
	TeamID string `json:"team_id"`
	Name   string `json:"name"`
}

type versionResp struct {
	ID          string `json:"id"`
	Version     string `json:"version"`
	Size        int64  `json:"size"`
	Sha256      string `json:"sha256"`
	ContentType string `json:"content_type"`
	Publisher   string `json:"publisher_user_id"`
	CreatedAt   string `json:"created_at"`
}

func (h *SkillHandlers) Publish(c *gin.Context) {
	t, ok := auth.CurrentTeam(c)
	if !ok {
		c.Error(apperr.New("not_found", "team", "no team"))
		return
	}
	u, ok := auth.CurrentUser(c)
	if !ok {
		c.Error(apperr.New("unauthorized", "auth", "no user"))
		return
	}
	if !h.teamSvc.CanPublish(c.Request.Context(), t, u.ID) {
		c.Error(apperr.New("forbidden", "skill", "forbidden"))
		c.AbortWithStatus(http.StatusForbidden)
		return
	}
	name := c.Param("name")
	version := c.Param("version")
	size, _ := strconv.ParseInt(c.Request.Header.Get("Content-Length"), 10, 64)
	sv, err := h.svc.Publish(c.Request.Context(), t.ID, name, version, c.Request.Body, size, c.ContentType(), u.ID)
	if err != nil {
		c.Error(err)
		return
	}
	c.JSON(http.StatusCreated, versionResp{
		ID:          sv.ID.String(),
		Version:     sv.Version,
		Size:        sv.Size,
		Sha256:      sv.Sha256,
		ContentType: sv.ContentType,
		Publisher:   sv.PublisherUserID.String(),
		CreatedAt:   sv.CreatedAt.Format(timeRFC3339),
	})
}

func (h *SkillHandlers) ListSkills(c *gin.Context) {
	t, _ := auth.CurrentTeam(c)
	ss, err := h.svc.ListSkillsByTeam(c.Request.Context(), t.ID)
	if err != nil {
		c.Error(err)
		return
	}
	out := make([]skillResp, len(ss))
	for i, s := range ss {
		out[i] = skillResp{ID: s.ID.String(), TeamID: s.TeamID.String(), Name: s.Name}
	}
	c.JSON(http.StatusOK, gin.H{"items": out})
}

func (h *SkillHandlers) GetSkill(c *gin.Context) {
	t, _ := auth.CurrentTeam(c)
	sk, vs, err := h.svc.GetSkillWithVersions(c.Request.Context(), t.ID, c.Param("name"))
	if err != nil {
		c.Error(err)
		return
	}
	out := make([]versionResp, len(vs))
	for i, v := range vs {
		out[i] = versionResp{
			ID:          v.ID.String(),
			Version:     v.Version,
			Size:        v.Size,
			Sha256:      v.Sha256,
			ContentType: v.ContentType,
			Publisher:   v.PublisherUserID.String(),
			CreatedAt:   v.CreatedAt.Format(timeRFC3339),
		}
	}
	c.JSON(http.StatusOK, gin.H{"id": sk.ID.String(), "team_id": sk.TeamID.String(), "name": sk.Name, "versions": out})
}

func (h *SkillHandlers) Download(c *gin.Context) {
	t, _ := auth.CurrentTeam(c)
	rc, sv, err := h.svc.OpenVersion(c.Request.Context(), t.ID, c.Param("name"), c.Param("version"))
	if err != nil {
		c.Error(err)
		return
	}
	defer rc.Close()
	c.Header("Content-Type", sv.ContentType)
	c.Header("Content-Length", strconv.FormatInt(sv.Size, 10))
	c.Header("Content-Disposition", `attachment; filename="`+c.Param("name")+"-"+c.Param("version")+`.tar.gz"`)
	c.Header("X-Skillhub-Sha256", sv.Sha256)
	c.Status(http.StatusOK)
	_, _ = io.Copy(c.Writer, rc)
}

type searchResultResp struct {
	ID            string       `json:"id"`
	TeamID        string       `json:"team_id"`
	TeamSlug      string       `json:"team_slug"`
	Name          string       `json:"name"`
	LatestVersion *versionResp `json:"latest_version"`
}

// Search handles GET /skills?q=&page=&page_size= — returns skills visible to
// the caller (global namespace + teams they own or belong to).
func (h *SkillHandlers) Search(c *gin.Context) {
	u, ok := auth.CurrentUser(c)
	if !ok {
		c.Error(apperr.New("unauthorized", "auth", "no user"))
		return
	}
	q := c.Query("q")
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))

	ctx := c.Request.Context()
	// 可见 teamIDs：global + 用户所属团队
	teamIDs := make([]uuid.UUID, 0, 4)
	if g, err := h.teamSvc.Repo().GetBySlug(ctx, team.GlobalSlug); err == nil {
		teamIDs = append(teamIDs, g.ID)
	}
	if mine, err := h.teamSvc.Repo().ListForUser(ctx, u.ID); err == nil {
		for _, t := range mine {
			teamIDs = append(teamIDs, t.ID)
		}
	}

	results, err := h.svc.Search(ctx, teamIDs, q, page, pageSize)
	if err != nil {
		c.Error(err)
		return
	}
	out := make([]searchResultResp, len(results))
	for i, r := range results {
		out[i] = searchResultResp{ID: r.ID.String(), TeamID: r.TeamID.String(), TeamSlug: r.TeamSlug, Name: r.Name}
		if r.LatestVersion != nil {
			out[i].LatestVersion = &versionResp{
				ID:          r.LatestVersion.ID.String(),
				Version:     r.LatestVersion.Version,
				Size:        r.LatestVersion.Size,
				Sha256:      r.LatestVersion.Sha256,
				ContentType: r.LatestVersion.ContentType,
				Publisher:   r.LatestVersion.PublisherUserID.String(),
				CreatedAt:   r.LatestVersion.CreatedAt.Format(timeRFC3339),
			}
		}
	}
	c.JSON(http.StatusOK, gin.H{"items": out, "page": page, "page_size": pageSize})
}
