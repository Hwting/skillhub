"use client";

import { useEffect, useState } from "react";
import Link from "next/link";
import { Plus, Users } from "lucide-react";
import { Button, buttonVariants } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { PageHeader } from "@/components/page-header";
import { EmptyState } from "@/components/empty-state";
import { TableSkeleton } from "@/components/skeletons";
import { AuthGuard } from "@/components/auth-guard";
import { TeamCreateDialog } from "@/components/team-create-dialog";
import { teamApi } from "@/lib/api";
import { ApiError, type Team } from "@/lib/types";
import { cn } from "@/lib/utils";

function TeamsBody() {
  const [teams, setTeams] = useState<Team[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [createOpen, setCreateOpen] = useState(false);

  function load() {
    setLoading(true);
    teamApi
      .list()
      .then((res) => setTeams(res.items))
      .catch((err) => setError(err instanceof ApiError ? err.message : "加载失败"))
      .finally(() => setLoading(false));
  }

  useEffect(load, []);

  return (
    <div className="mx-auto max-w-3xl px-4 py-10">
      <PageHeader
        title="我的团队"
        description="你拥有或所属的团队"
        className="mb-6"
        actions={
          <Button onClick={() => setCreateOpen(true)}>
            <Plus className="size-4" />
            新建团队
          </Button>
        }
      />
      {error && <p className="mb-4 text-sm text-destructive">{error}</p>}
      {loading ? (
        <TableSkeleton rows={4} cols={3} />
      ) : teams.length === 0 ? (
        <EmptyState
          icon={Users}
          title="还没有团队"
          description="创建一个团队来开始发布 skill"
          action={
            <Button size="sm" onClick={() => setCreateOpen(true)}>
              <Plus className="size-4" />
              新建团队
            </Button>
          }
        />
      ) : (
        <div className="rounded-xl border">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Slug</TableHead>
                <TableHead>名称</TableHead>
                <TableHead>发布策略</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {teams.map((t) => (
                <TableRow key={t.id}>
                  <TableCell>
                    <Link
                      href={`/teams/${t.slug}`}
                      className={cn(
                        buttonVariants({ variant: "link" }),
                        "h-auto px-0 font-medium",
                      )}
                    >
                      {t.slug}
                    </Link>
                  </TableCell>
                  <TableCell>{t.name}</TableCell>
                  <TableCell>
                    <Badge variant="outline">{t.publish_policy}</Badge>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </div>
      )}
      <TeamCreateDialog open={createOpen} onOpenChange={setCreateOpen} onCreated={() => load()} />
    </div>
  );
}

export default function TeamsPage() {
  return (
    <AuthGuard>
      <TeamsBody />
    </AuthGuard>
  );
}
