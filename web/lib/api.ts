import { ApiError, type ApiErrorBody, type ListResponse, type Paginated, type Team, type Member, type User, type SearchResult, type SkillSummary, type SkillDetail, type Version } from "./types";

// All requests go to /api/* which next.config.ts rewrites to the Go backend.
// The sid cookie is httpOnly; the browser attaches it automatically for same-origin
// requests, so default credentials:"same-origin" suffices.

// Injected by UserProvider to avoid a circular import. Called once on a 401.
let onUnauthorized: (() => void) | null = null;
export function setOnUnauthorized(cb: (() => void) | null) {
  onUnauthorized = cb;
}

async function apiFetch<T>(path: string, init?: RequestInit): Promise<T> {
  // Auto-add JSON content-type for string bodies (JSON.stringify result);
  // binary callers (publish) pass their own Content-Type.
  const headers = new Headers(init?.headers);
  if (init?.body && typeof init.body === "string" && !headers.has("Content-Type")) {
    headers.set("Content-Type", "application/json");
  }

  const res = await fetch(`/api${path}`, {
    credentials: "same-origin",
    ...init,
    headers,
  });

  if (res.status === 401 && onUnauthorized) {
    onUnauthorized();
  }

  if (res.status === 204) {
    return null as T;
  }

  if (!res.ok) {
    let body: ApiErrorBody | null = null;
    try {
      body = (await res.json()) as ApiErrorBody;
    } catch {
      // non-JSON error
    }
    if (body?.error) {
      throw new ApiError(res.status, body.error.code, body.error.message, body.error.request_id);
    }
    throw new ApiError(res.status, "internal", `request failed: ${res.status}`);
  }

  return (await res.json()) as T;
}

function jsonBody(body: unknown): RequestInit {
  return { method: "POST", body: JSON.stringify(body) };
}

// --- auth ---
export const authApi = {
  register: (body: { email: string; username: string; password: string }) =>
    apiFetch<User>("/register", jsonBody(body)),
  login: (body: { email: string; password: string }) =>
    apiFetch<User>("/login", jsonBody(body)),
  logout: () => apiFetch<null>("/logout", { method: "POST" }),
  me: () => apiFetch<User>("/me"),
};

// --- teams ---
export const teamApi = {
  list: () => apiFetch<ListResponse<Team>>("/teams"),
  get: (slug: string) => apiFetch<Team>(`/teams/${slug}`),
  create: (body: { slug: string; name: string }) =>
    apiFetch<Team>("/teams", jsonBody(body)),
  patch: (slug: string, body: { name?: string; publish_policy?: string }) =>
    apiFetch<null>(`/teams/${slug}`, { method: "PATCH", body: JSON.stringify(body) }),
  delete: (slug: string) => apiFetch<null>(`/teams/${slug}`, { method: "DELETE" }),
  listMembers: (slug: string) =>
    apiFetch<ListResponse<Member>>(`/teams/${slug}/members`),
  addMember: (slug: string, body: { user_id: string; role: string }) =>
    apiFetch<null>(`/teams/${slug}/members`, jsonBody(body)),
  patchMember: (slug: string, uid: string, body: { role: string }) =>
    apiFetch<null>(`/teams/${slug}/members/${uid}`, { method: "PATCH", body: JSON.stringify(body) }),
  removeMember: (slug: string, uid: string) =>
    apiFetch<null>(`/teams/${slug}/members/${uid}`, { method: "DELETE" }),
  transfer: (slug: string, body: { new_owner_id: string }) =>
    apiFetch<null>(`/teams/${slug}/transfer`, jsonBody(body)),
};

// --- skills ---
export const skillApi = {
  search: (params: { q?: string; page?: number; page_size?: number }) => {
    const sp = new URLSearchParams();
    if (params.q) sp.set("q", params.q);
    if (params.page) sp.set("page", String(params.page));
    if (params.page_size) sp.set("page_size", String(params.page_size));
    return apiFetch<Paginated<SearchResult>>(`/skills?${sp.toString()}`);
  },
  listByTeam: (slug: string) =>
    apiFetch<ListResponse<SkillSummary>>(`/teams/${slug}/skills`),
  detail: (slug: string, name: string) =>
    apiFetch<SkillDetail>(`/teams/${slug}/skills/${name}`),
  star: (slug: string, name: string) =>
    apiFetch<null>(`/teams/${slug}/skills/${name}/star`, { method: "POST" }),
  unstar: (slug: string, name: string) =>
    apiFetch<null>(`/teams/${slug}/skills/${name}/star`, { method: "DELETE" }),
  myStars: (params: { page?: number; page_size?: number }) => {
    const sp = new URLSearchParams();
    if (params.page) sp.set("page", String(params.page));
    if (params.page_size) sp.set("page_size", String(params.page_size));
    return apiFetch<Paginated<SearchResult>>(`/me/stars?${sp.toString()}`);
  },
  // Publish a raw tarball. Caller passes the ArrayBuffer; do not set
  // Content-Length manually — fetch computes it.
  publish: (slug: string, name: string, version: string, body: ArrayBuffer) =>
    apiFetch<Version>(`/teams/${slug}/skills/${name}/versions/${version}`, {
      method: "POST",
      headers: { "Content-Type": "application/gzip" },
      body,
    }),
};

export type { Team, Member, User, SearchResult, SkillSummary, SkillDetail, Version };
