"use client";

import { useEffect, useState } from "react";
import Link from "next/link";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { skillApi } from "@/lib/api";
import { ApiError, type SkillSummary } from "@/lib/types";

export function TeamSkillsTab({ slug }: { slug: string }) {
  const [skills, setSkills] = useState<SkillSummary[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    skillApi
      .listByTeam(slug)
      .then((res) => setSkills(res.items))
      .catch((err) => setError(err instanceof ApiError ? err.message : "加载失败"))
      .finally(() => setLoading(false));
  }, [slug]);

  if (loading) return <p className="text-muted-foreground">加载中…</p>;
  if (error) return <p className="text-destructive">{error}</p>;
  if (skills.length === 0) return <p className="text-muted-foreground">该团队还没有 skill</p>;

  return (
    <Table>
      <TableHeader>
        <TableRow>
          <TableHead>名称</TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {skills.map((s) => (
          <TableRow key={s.id}>
            <TableCell>
              <Link href={`/teams/${slug}/skills/${s.name}`} className="text-primary underline">
                {s.name}
              </Link>
            </TableCell>
          </TableRow>
        ))}
      </TableBody>
    </Table>
  );
}
