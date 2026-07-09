"use client";

import { useEffect, useState } from "react";
import { SkillListItem } from "@/components/skill-list-item";
import { Pagination } from "@/components/pagination";
import { AuthGuard } from "@/components/auth-guard";
import { skillApi } from "@/lib/api";
import { ApiError, type SearchResult } from "@/lib/types";

const PAGE_SIZE = 20;

function StarsBody() {
  const [page, setPage] = useState(1);
  const [items, setItems] = useState<SearchResult[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    setLoading(true);
    setError(null);
    skillApi
      .myStars({ page, page_size: PAGE_SIZE })
      .then((res) => setItems(res.items))
      .catch((err) => setError(err instanceof ApiError ? err.message : "加载失败"))
      .finally(() => setLoading(false));
  }, [page]);

  return (
    <div className="mx-auto max-w-3xl px-4 py-8">
      <h1 className="mb-4 text-xl font-semibold">我收藏的 skill</h1>
      {error && <p className="text-sm text-destructive">{error}</p>}
      {loading ? (
        <p className="text-muted-foreground">加载中…</p>
      ) : items.length === 0 ? (
        <p className="text-muted-foreground">还没有收藏</p>
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

export default function StarsPage() {
  return (
    <AuthGuard>
      <StarsBody />
    </AuthGuard>
  );
}
