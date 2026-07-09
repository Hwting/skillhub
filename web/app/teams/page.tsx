"use client";

import { useEffect, useState } from "react";
import Link from "next/link";
import { Button } from "@/components/ui/button";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { AuthGuard } from "@/components/auth-guard";
import { TeamCreateDialog } from "@/components/team-create-dialog";
import { teamApi } from "@/lib/api";
import { ApiError, type Team } from "@/lib/types";

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
    <div className="mx-auto max-w-3xl px-4 py-8">
      <div className="mb-4 flex items-center justify-between">
        <h1 className="text-xl font-semibold">我的团队</h1>
        <Button onClick={() => setCreateOpen(true)}>新建团队</Button>
      </div>
      {error && <p className="text-sm text-destructive">{error}</p>}
      {loading ? (
        <p className="text-muted-foreground">加载中…</p>
      ) : teams.length === 0 ? (
        <p className="text-muted-foreground">还没有团队</p>
      ) : (
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
                  <Link href={`/teams/${t.slug}`} className="text-primary underline">
                    {t.slug}
                  </Link>
                </TableCell>
                <TableCell>{t.name}</TableCell>
                <TableCell>{t.publish_policy}</TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
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
