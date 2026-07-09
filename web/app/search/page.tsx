"use client";

import { useEffect, useState } from "react";
import { Input } from "@/components/ui/input";
import { SkillListItem } from "@/components/skill-list-item";
import { Pagination } from "@/components/pagination";
import { AuthGuard } from "@/components/auth-guard";
import { skillApi } from "@/lib/api";
import { ApiError, type SearchResult } from "@/lib/types";

const PAGE_SIZE = 20;

function SearchBody() {
  const [q, setQ] = useState("");
  const [debounced, setDebounced] = useState("");
  const [page, setPage] = useState(1);
  const [items, setItems] = useState<SearchResult[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  // debounce the query input
  useEffect(() => {
    const t = setTimeout(() => {
      setDebounced(q);
      setPage(1);
    }, 300);
    return () => clearTimeout(t);
  }, [q]);

  useEffect(() => {
    setLoading(true);
    setError(null);
    skillApi
      .search({ q: debounced, page, page_size: PAGE_SIZE })
      .then((res) => setItems(res.items))
      .catch((err) => setError(err instanceof ApiError ? err.message : "搜索失败"))
      .finally(() => setLoading(false));
  }, [debounced, page]);

  return (
    <div className="mx-auto max-w-3xl px-4 py-8">
      <h1 className="mb-4 text-xl font-semibold">搜索 skill</h1>
      <Input
        placeholder="按名称搜索…"
        value={q}
        onChange={(e) => setQ(e.target.value)}
        className="mb-4"
      />
      {error && <p className="text-sm text-destructive">{error}</p>}
      {loading ? (
        <p className="text-muted-foreground">加载中…</p>
      ) : items.length === 0 ? (
        <p className="text-muted-foreground">无结果</p>
      ) : (
        <div className="flex flex-col gap-2">
          {items.map((s) => (
            <SkillListItem key={s.id} skill={s} />
          ))}
        </div>
      )}
      <Pagination
        page={page}
        pageSize={PAGE_SIZE}
        hasMore={items.length === PAGE_SIZE}
        onPageChange={setPage}
      />
    </div>
  );
}

export default function SearchPage() {
  return (
    <AuthGuard>
      <SearchBody />
    </AuthGuard>
  );
}
