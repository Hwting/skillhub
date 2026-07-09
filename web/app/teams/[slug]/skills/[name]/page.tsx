"use client";

import { useEffect, useState } from "react";
import { useParams } from "next/navigation";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { AuthGuard } from "@/components/auth-guard";
import { StarButton } from "@/components/star-button";
import { PublishDialog } from "@/components/publish-dialog";
import { skillApi } from "@/lib/api";
import { ApiError, type SkillDetail } from "@/lib/types";
import { downloadText } from "@/lib/download";

function formatSize(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KiB`;
  return `${(bytes / 1024 / 1024).toFixed(1)} MiB`;
}

function SkillDetailBody() {
  const params = useParams<{ slug: string; name: string }>();
  const { slug, name } = params;
  const [detail, setDetail] = useState<SkillDetail | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [publishOpen, setPublishOpen] = useState(false);
  const [exporting, setExporting] = useState(false);

  function load() {
    setLoading(true);
    skillApi
      .detail(slug, name)
      .then(setDetail)
      .catch((err) => setError(err instanceof ApiError ? err.message : "加载失败"))
      .finally(() => setLoading(false));
  }
  useEffect(load, [slug, name]);

  async function onExport() {
    setExporting(true);
    try {
      const manifest = await skillApi.exportManifest(slug, name);
      downloadText(`${name}.manifest.json`, JSON.stringify(manifest, null, 2));
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "导出失败");
    } finally {
      setExporting(false);
    }
  }

  if (loading) return <p className="mx-auto max-w-3xl px-4 py-8 text-muted-foreground">加载中…</p>;
  if (error) return <p className="mx-auto max-w-3xl px-4 py-8 text-destructive">{error}</p>;
  if (!detail) return null;

  return (
    <div className="mx-auto max-w-3xl px-4 py-8">
      <div className="mb-6 flex items-center justify-between">
        <div>
          <h1 className="text-xl font-semibold">{detail.name}</h1>
          <p className="text-sm text-muted-foreground">{slug}</p>
        </div>
        <div className="flex items-center gap-3">
          <Badge variant="secondary">★ {detail.star_count}</Badge>
          <StarButton slug={slug} name={name} starred={detail.is_starred} onToggled={load} />
          <Button variant="outline" onClick={onExport} disabled={exporting}>
            {exporting ? "导出中…" : "导出清单"}
          </Button>
          <Button onClick={() => setPublishOpen(true)}>发布新版本</Button>
        </div>
      </div>

      {detail.versions.length === 0 ? (
        <p className="text-muted-foreground">还没有版本</p>
      ) : (
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>版本</TableHead>
              <TableHead>大小</TableHead>
              <TableHead>Sha256</TableHead>
              <TableHead>发布者</TableHead>
              <TableHead>操作</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {detail.versions.map((v) => (
              <TableRow key={v.id}>
                <TableCell className="font-medium">{v.version}</TableCell>
                <TableCell>{formatSize(v.size)}</TableCell>
                <TableCell className="font-mono text-xs text-muted-foreground">{v.sha256.slice(0, 12)}…</TableCell>
                <TableCell className="font-mono text-xs text-muted-foreground">{v.publisher_user_id.slice(0, 8)}…</TableCell>
                <TableCell>
                  <a
                    href={`/api/teams/${slug}/skills/${name}/versions/${v.version}`}
                    download={`${name}-${v.version}.tar.gz`}
                  >
                    <Button size="sm" variant="outline">
                      下载
                    </Button>
                  </a>
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      )}

      <PublishDialog
        slug={slug}
        name={name}
        open={publishOpen}
        onOpenChange={setPublishOpen}
        onPublished={load}
      />
    </div>
  );
}

export default function SkillDetailPage() {
  return (
    <AuthGuard>
      <SkillDetailBody />
    </AuthGuard>
  );
}
