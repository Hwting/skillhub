"use client";

import { useEffect, useState } from "react";
import { Search } from "lucide-react";
import { Input } from "@/components/ui/input";
import { SkillListItem } from "@/components/skill-list-item";
import { Pagination } from "@/components/pagination";
import { PageHeader } from "@/components/page-header";
import { EmptyState } from "@/components/empty-state";
import { CardListSkeleton } from "@/components/skeletons";
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
    <div className="mx-auto max-w-3xl px-4 py-10">
      <PageHeader
        title="搜索 skill"
        description="跨所有可见团队与全局命名空间查找"
        className="mb-6"
      />
      <div className="relative mb-6">
        <Search className="absolute left-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
        <Input
          placeholder="按名称搜索…"
          value={q}
          onChange={(e) => setQ(e.target.value)}
          className="pl-9"
        />
      </div>
      {error && <p className="mb-4 text-sm text-destructive">{error}</p>}
      {loading ? (
        <CardListSkeleton />
      ) : items.length === 0 ? (
        <EmptyState
          icon={Search}
          title="无结果"
          description={debounced ? `没有匹配「${debounced}」的 skill` : "还没有可见的 skill"}
        />
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
