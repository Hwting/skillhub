CREATE TABLE skills (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    team_id    UUID NOT NULL REFERENCES teams(id) ON DELETE CASCADE,
    name       TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (team_id, name)
);
CREATE INDEX skills_team_idx ON skills(team_id);

CREATE TABLE skill_versions (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    skill_id          UUID NOT NULL REFERENCES skills(id) ON DELETE CASCADE,
    version           TEXT NOT NULL,
    storage_key       TEXT NOT NULL,
    size              BIGINT NOT NULL,
    sha256            TEXT NOT NULL,
    content_type      TEXT NOT NULL,
    publisher_user_id UUID NOT NULL REFERENCES users(id),
    readme            TEXT,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (skill_id, version)
);
CREATE INDEX skill_versions_skill_idx ON skill_versions(skill_id);
