"use client";

import { useEffect, useState } from "react";
import { useParams, useRouter } from "next/navigation";
import { Star, Download, FileJson, Upload, PackageX, Trash } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { PageHeader } from "@/components/page-header";
import { EmptyState } from "@/components/empty-state";
import { AuthGuard } from "@/components/auth-guard";
import { StarButton } from "@/components/star-button";
import { PublishDialog } from "@/components/publish-dialog";
import { DeleteSkillDialog } from "@/components/delete-skill-dialog";
import { toast } from "sonner";
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
  const router = useRouter();
  const [deleteOpen, setDeleteOpen] = useState(false);

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

  if (loading) return <p className="mx-auto max-w-3xl px-4 py-10 text-muted-foreground">加载中…</p>;
  if (error) return <p className="mx-auto max-w-3xl px-4 py-10 text-destructive">{error}</p>;
  if (!detail) return null;

  return (
    <div className="mx-auto max-w-3xl px-4 py-10">
      <PageHeader
        title={detail.name}
        description={slug}
        className="mb-6"
        actions={
          <div className="flex items-center gap-2">
            <Badge variant="secondary" className="gap-1">
              <Star className="size-3" />
              {detail.star_count}
            </Badge>
            <StarButton slug={slug} name={name} starred={detail.is_starred} onToggled={load} />
            <Button variant="outline" onClick={onExport} disabled={exporting}>
              <FileJson className="size-4" />
              {exporting ? "导出中…" : "清单"}
            </Button>
            <Button onClick={() => setPublishOpen(true)}>
              <Upload className="size-4" />
              发布新版本
            </Button>
            <Button variant="outline" onClick={() => setDeleteOpen(true)}>
              <Trash className="size-4" />
              删除
            </Button>
          </div>
        }
      />

      {detail.versions.length === 0 ? (
        <EmptyState
          icon={PackageX}
          title="还没有版本"
          description="发布第一个版本吧"
          action={
            <Button size="sm" onClick={() => setPublishOpen(true)}>
              <Upload className="size-4" />
              发布版本
            </Button>
          }
        />
      ) : (
        <div className="rounded-xl border">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>版本</TableHead>
                <TableHead>大小</TableHead>
                <TableHead>Sha256</TableHead>
                <TableHead>发布者</TableHead>
                <TableHead className="text-right">操作</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {detail.versions.map((v) => (
                <TableRow key={v.id}>
                  <TableCell className="font-medium">{v.version}</TableCell>
                  <TableCell className="text-muted-foreground">{formatSize(v.size)}</TableCell>
                  <TableCell className="font-mono text-xs text-muted-foreground">{v.sha256.slice(0, 12)}…</TableCell>
                  <TableCell className="font-mono text-xs text-muted-foreground">{v.publisher_user_id.slice(0, 8)}…</TableCell>
                  <TableCell className="text-right">
                    <a
                      href={`/api/teams/${slug}/skills/${name}/versions/${v.version}`}
                      download={`${name}-${v.version}.tar.gz`}
                    >
                      <Button size="sm" variant="outline">
                        <Download className="size-3.5" />
                        下载
                      </Button>
                    </a>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </div>
      )}

      <PublishDialog
        slug={slug}
        name={name}
        open={publishOpen}
        onOpenChange={setPublishOpen}
        onPublished={load}
      />
      <DeleteSkillDialog
        slug={slug}
        name={name}
        open={deleteOpen}
        onOpenChange={setDeleteOpen}
        onDeleted={() => {
          toast("已删除");
          router.push(`/teams/${slug}`);
        }}
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
