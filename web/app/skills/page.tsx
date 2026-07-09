"use client";

import { useEffect, useState } from "react";
import Link from "next/link";
import { Search, Package, Download, FileJson, Eye } from "lucide-react";
import { Input } from "@/components/ui/input";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { Pagination } from "@/components/pagination";
import { PageHeader } from "@/components/page-header";
import { EmptyState } from "@/components/empty-state";
import { TableSkeleton } from "@/components/skeletons";
import { AuthGuard } from "@/components/auth-guard";
import { skillApi } from "@/lib/api";
import { ApiError, type SearchResult } from "@/lib/types";
import { downloadText } from "@/lib/download";

const PAGE_SIZE = 20;

function formatSize(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KiB`;
  return `${(bytes / 1024 / 1024).toFixed(1)} MiB`;
}

function SkillsManageBody() {
  const [q, setQ] = useState("");
  const [debounced, setDebounced] = useState("");
  const [page, setPage] = useState(1);
  const [items, setItems] = useState<SearchResult[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [exporting, setExporting] = useState<string | null>(null);

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
      .catch((err) => setError(err instanceof ApiError ? err.message : "加载失败"))
      .finally(() => setLoading(false));
  }, [debounced, page]);

  async function onExport(slug: string, name: string) {
    const key = `${slug}/${name}`;
    setExporting(key);
    try {
      const manifest = await skillApi.exportManifest(slug, name);
      downloadText(`${name}.manifest.json`, JSON.stringify(manifest, null, 2));
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "导出失败");
    } finally {
      setExporting(null);
    }
  }

  return (
    <div className="mx-auto max-w-5xl px-4 py-10">
      <PageHeader
        title="技能管理"
        description="你可见的全部 skill — 下载最新版本或导出清单"
        className="mb-6"
      />
      <div className="relative mb-6">
        <Search className="absolute left-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
        <Input
          placeholder="过滤名称…"
          value={q}
          onChange={(e) => setQ(e.target.value)}
          className="pl-9"
        />
      </div>
      {error && <p className="mb-4 text-sm text-destructive">{error}</p>}
      {loading ? (
        <TableSkeleton rows={6} cols={5} />
      ) : items.length === 0 ? (
        <EmptyState
          icon={Package}
          title="没有可见的 skill"
          description="去发布页创建你的第一个 skill"
          action={
            <Button size="sm" nativeButton={false} render={<Link href="/publish" />}>
              发布 skill
            </Button>
          }
        />
      ) : (
        <div className="rounded-xl border">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>名称</TableHead>
                <TableHead>团队</TableHead>
                <TableHead>最新版本</TableHead>
                <TableHead>大小</TableHead>
                <TableHead className="text-right">操作</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {items.map((s) => {
                const key = `${s.team_slug}/${s.name}`;
                const lv = s.latest_version;
                return (
                  <TableRow key={s.id}>
                    <TableCell className="font-medium">{s.name}</TableCell>
                    <TableCell>
                      <Badge variant="secondary">{s.team_slug}</Badge>
                    </TableCell>
                    <TableCell>{lv ? lv.version : "—"}</TableCell>
                    <TableCell className="text-muted-foreground">
                      {lv ? formatSize(lv.size) : "—"}
                    </TableCell>
                    <TableCell>
                      <div className="flex items-center justify-end gap-1.5">
                        {lv ? (
                          <a
                            href={`/api/teams/${s.team_slug}/skills/${s.name}/versions/${lv.version}`}
                            download={`${s.name}-${lv.version}.tar.gz`}
                          >
                            <Button size="sm" variant="outline">
                              <Download className="size-3.5" />
                              下载
                            </Button>
                          </a>
                        ) : (
                          <Button size="sm" variant="outline" disabled>
                            <Download className="size-3.5" />
                            下载
                          </Button>
                        )}
                        <Button
                          size="sm"
                          variant="outline"
                          disabled={exporting === key}
                          onClick={() => onExport(s.team_slug, s.name)}
                        >
                          <FileJson className="size-3.5" />
                          {exporting === key ? "导出中…" : "清单"}
                        </Button>
                        <Button size="sm" variant="ghost" nativeButton={false} render={<Link href={`/teams/${s.team_slug}/skills/${s.name}`} />}>
                          <Eye className="size-3.5" />
                        </Button>
                      </div>
                    </TableCell>
                  </TableRow>
                );
              })}
            </TableBody>
          </Table>
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

export default function SkillsManagePage() {
  return (
    <AuthGuard>
      <SkillsManageBody />
    </AuthGuard>
  );
}
