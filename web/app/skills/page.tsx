"use client";

import { useEffect, useState } from "react";
import Link from "next/link";
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
    <div className="mx-auto max-w-5xl px-4 py-8">
      <h1 className="mb-4 text-xl font-semibold">技能管理</h1>
      <Input
        placeholder="过滤名称…"
        value={q}
        onChange={(e) => setQ(e.target.value)}
        className="mb-4"
      />
      {error && <p className="mb-4 text-sm text-destructive">{error}</p>}
      {loading ? (
        <p className="text-muted-foreground">加载中…</p>
      ) : items.length === 0 ? (
        <p className="text-muted-foreground">没有可见的 skill</p>
      ) : (
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>名称</TableHead>
              <TableHead>团队</TableHead>
              <TableHead>最新版本</TableHead>
              <TableHead>大小</TableHead>
              <TableHead>操作</TableHead>
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
                    <div className="flex items-center gap-2">
                      {lv ? (
                        <a
                          href={`/api/teams/${s.team_slug}/skills/${s.name}/versions/${lv.version}`}
                          download={`${s.name}-${lv.version}.tar.gz`}
                        >
                          <Button size="sm" variant="outline">
                            下载
                          </Button>
                        </a>
                      ) : (
                        <Button size="sm" variant="outline" disabled>
                          下载
                        </Button>
                      )}
                      <Button
                        size="sm"
                        variant="outline"
                        disabled={exporting === key}
                        onClick={() => onExport(s.team_slug, s.name)}
                      >
                        {exporting === key ? "导出中…" : "导出清单"}
                      </Button>
                      <Link href={`/teams/${s.team_slug}/skills/${s.name}`}>
                        <Button size="sm" variant="ghost">
                          查看
                        </Button>
                      </Link>
                    </div>
                  </TableCell>
                </TableRow>
              );
            })}
          </TableBody>
        </Table>
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
