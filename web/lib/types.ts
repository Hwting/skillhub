// Domain types mirroring the SkillHub backend JSON shapes.

export interface User {
  id: string;
  email: string;
  username: string;
  role: string;
  status: string;
}

export interface Team {
  id: string;
  slug: string;
  name: string;
  owner_user_id: string; // "" when null (global namespace)
  publish_policy: "admin_only" | "any_member";
}

export interface Member {
  user_id: string;
  role: "owner" | "admin" | "member";
  created_at: string; // RFC3339
}

export interface Version {
  id: string;
  version: string;
  size: number;
  sha256: string;
  content_type: string;
  publisher_user_id: string;
  created_at: string; // RFC3339
}

// GET /skills and /me/stars items.
export interface SearchResult {
  id: string;
  team_id: string;
  team_slug: string;
  name: string;
  latest_version: Version | null;
}

// GET /teams/:slug/skills items — smaller shape than SearchResult.
export interface SkillSummary {
  id: string;
  team_id: string;
  name: string;
}

// GET /teams/:slug/skills/:name
export interface SkillDetail {
  id: string;
  team_id: string;
  name: string;
  versions: Version[];
  star_count: number;
  is_starred: boolean;
}

export interface Paginated<T> {
  items: T[];
  page: number;
  page_size: number;
}

export interface ListResponse<T> {
  items: T[];
}

// Backend error body: {error:{code,message,request_id}}.
export interface ApiErrorBody {
  error: {
    code: string;
    message: string;
    request_id?: string;
  };
}

export class ApiError extends Error {
  readonly code: string;
  readonly request_id?: string;
  readonly status: number;

  constructor(status: number, code: string, message: string, request_id?: string) {
    super(message);
    this.name = "ApiError";
    this.status = status;
    this.code = code;
    this.request_id = request_id;
  }
}
