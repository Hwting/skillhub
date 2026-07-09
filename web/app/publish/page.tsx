"use client";

import { useEffect, useState } from "react";
import { useRouter } from "next/navigation";
import Link from "next/link";
import { Upload, Users } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Card, CardContent, CardHeader, CardTitle, CardDescription, CardAction } from "@/components/ui/card";
import { PageHeader } from "@/components/page-header";
import { EmptyState } from "@/components/empty-state";
import { AuthGuard } from "@/components/auth-guard";
import { PackageInput } from "@/components/package-input";
import { teamApi, skillApi } from "@/lib/api";
import { ApiError, type Team } from "@/lib/types";
import { buildPackageBody, type PackageSelection } from "@/lib/tgz";

const selectClass =
  "flex h-9 w-full rounded-md border border-input bg-transparent px-3 py-1 text-sm shadow-sm transition-colors focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring";

function PublishBody() {
  const router = useRouter();
  const [teams, setTeams] = useState<Team[]>([]);
  const [slug, setSlug] = useState("");
  const [name, setName] = useState("");
  const [version, setVersion] = useState("");
  const [pkg, setPkg] = useState<PackageSelection | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);

  useEffect(() => {
    teamApi
      .list()
      .then((res) => {
        setTeams(res.items);
        if (res.items[0]) setSlug(res.items[0].slug);
      })
      .catch((err) => setError(err instanceof ApiError ? err.message : "加载团队失败"))
      .finally(() => setLoading(false));
  }, []);

  async function onSubmit(e: React.FormEvent) {
    e.preventDefault();
    setError(null);
    if (!pkg) {
      setError("请选择上传内容");
      return;
    }
    let body: ArrayBuffer;
    try {
      body = await buildPackageBody(pkg);
    } catch (err) {
      setError(err instanceof Error ? err.message : "打包失败");
      return;
    }
    setSubmitting(true);
    try {
      await skillApi.publish(slug, name, version, body);
      router.push(`/teams/${slug}/skills/${name}`);
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "发布失败");
    } finally {
      setSubmitting(false);
    }
  }

  if (loading) return <p className="mx-auto max-w-xl px-4 py-10 text-muted-foreground">加载中…</p>;

  return (
    <div className="mx-auto max-w-xl px-4 py-10">
      <PageHeader
        title="发布新 skill"
        description="选择团队与文件，浏览器就地打包上传"
        className="mb-6"
      />
      {teams.length === 0 ? (
        <EmptyState
          icon={Users}
          title="还没有可发布的团队"
          description="先创建一个团队，再回来发布"
          action={
            <Button size="sm" nativeButton={false} render={<Link href="/teams" />}>
              去创建团队
            </Button>
          }
        />
      ) : (
        <Card className="shadow-soft">
          <CardHeader className="border-b">
            <CardTitle className="flex items-center gap-2">
              <Upload className="size-4 text-muted-foreground" />
              发布详情
            </CardTitle>
            <CardDescription>首次发布会自动创建该 skill</CardDescription>
            <CardAction>
              <span className="font-mono text-[11px] text-muted-foreground/80">publish.yaml</span>
            </CardAction>
          </CardHeader>
          <CardContent>
            <form onSubmit={onSubmit} className="flex flex-col gap-4">
              <div className="flex flex-col gap-2">
                <Label htmlFor="team">团队</Label>
                <select
                  id="team"
                  value={slug}
                  onChange={(e) => setSlug(e.target.value)}
                  className={selectClass}
                  required
                >
                  {teams.map((t) => (
                    <option key={t.slug} value={t.slug}>
                      {t.name} ({t.slug})
                    </option>
                  ))}
                </select>
              </div>
              <div className="grid gap-4 sm:grid-cols-2">
                <div className="flex flex-col gap-2">
                  <Label htmlFor="name">Skill 名称</Label>
                  <Input
                    id="name"
                    value={name}
                    onChange={(e) => setName(e.target.value)}
                    required
                    placeholder="my-skill"
                    className="font-mono"
                  />
                </div>
                <div className="flex flex-col gap-2">
                  <Label htmlFor="version">版本号</Label>
                  <Input
                    id="version"
                    value={version}
                    onChange={(e) => setVersion(e.target.value)}
                    required
                    placeholder="1.0.0"
                    className="font-mono"
                  />
                </div>
              </div>
              <PackageInput onChange={setPkg} />
              {error && <p className="text-sm text-destructive">{error}</p>}
              <Button type="submit" disabled={submitting} className="w-full">
                {submitting ? "发布中…" : "发布"}
              </Button>
              <div className="flex items-center gap-2 rounded-md bg-muted/50 px-3 py-2">
                <span className="font-mono text-[11px] text-muted-foreground">→</span>
                <code className="truncate font-mono text-xs text-foreground/80">
                  teams/{slug || "team"}/skills/{name.trim() || "my-skill"}
                </code>
                <span className="font-mono text-[11px] text-muted-foreground">
                  · v{version.trim() || "1.0.0"}
                </span>
              </div>
              <p className="text-xs text-muted-foreground">
                若名称已存在则追加一个新版本（版本号不能重复）。
              </p>
            </form>
          </CardContent>
        </Card>
      )}
    </div>
  );
}

export default function PublishPage() {
  return (
    <AuthGuard>
      <PublishBody />
    </AuthGuard>
  );
}
