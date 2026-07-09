"use client";

import { useEffect, useState } from "react";
import { Star } from "lucide-react";
import { SkillListItem } from "@/components/skill-list-item";
import { Pagination } from "@/components/pagination";
import { PageHeader } from "@/components/page-header";
import { EmptyState } from "@/components/empty-state";
import { CardListSkeleton } from "@/components/skeletons";
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
    <div className="mx-auto max-w-3xl px-4 py-10">
      <PageHeader
        title="我收藏的 skill"
        description="你 star 过的 skill，按收藏时间倒序"
        className="mb-6"
      />
      {error && <p className="mb-4 text-sm text-destructive">{error}</p>}
      {loading ? (
        <CardListSkeleton />
      ) : items.length === 0 ? (
        <EmptyState
          icon={Star}
          title="还没有收藏"
          description="在 skill 详情页点击 ☆ 即可收藏"
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

export default function StarsPage() {
  return (
    <AuthGuard>
      <StarsBody />
    </AuthGuard>
  );
}
