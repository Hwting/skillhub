"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { teamApi } from "@/lib/api";
import { ApiError, type Team } from "@/lib/types";

interface Props {
  team: Team;
  isOwner: boolean;
  onTeamChanged: () => void;
}

export function TeamSettingsTab({ team, isOwner, onTeamChanged }: Props) {
  const router = useRouter();
  const [name, setName] = useState(team.name);
  const [policy, setPolicy] = useState(team.publish_policy);
  const [error, setError] = useState<string | null>(null);
  const [saving, setSaving] = useState(false);

  if (!isOwner) {
    return <p className="text-muted-foreground">仅 owner 可修改团队设置</p>;
  }

  async function save(e: React.FormEvent) {
    e.preventDefault();
    setSaving(true);
    setError(null);
    try {
      await teamApi.patch(team.slug, { name, publish_policy: policy });
      onTeamChanged();
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "保存失败");
    } finally {
      setSaving(false);
    }
  }

  async function del() {
    if (!confirm(`确认删除团队 ${team.slug}？此操作不可撤销。`)) return;
    try {
      await teamApi.delete(team.slug);
      router.push("/teams");
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "删除失败");
    }
  }

  return (
    <div className="flex flex-col gap-4">
      {error && <p className="text-sm text-destructive">{error}</p>}
      <form onSubmit={save} className="flex flex-col gap-4 rounded-lg border p-4">
        <div className="flex flex-col gap-2">
          <Label htmlFor="name">团队名称</Label>
          <Input id="name" value={name} onChange={(e) => setName(e.target.value)} required />
        </div>
        <div className="flex flex-col gap-2">
          <Label htmlFor="policy">发布策略</Label>
          <select
            id="policy"
            value={policy}
            onChange={(e) => setPolicy(e.target.value as "admin_only" | "any_member")}
            className="h-8 rounded-lg border px-2 text-sm"
          >
            <option value="admin_only">admin_only</option>
            <option value="any_member">any_member</option>
          </select>
        </div>
        <Button type="submit" disabled={saving} className="w-fit">
          {saving ? "保存中…" : "保存"}
        </Button>
      </form>

      {team.slug !== "global" && (
        <div className="rounded-lg border border-destructive/30 p-4">
          <p className="mb-2 text-sm text-muted-foreground">删除团队将一并删除其下所有 skill 与版本。</p>
          <Button variant="destructive" onClick={del}>
            删除团队
          </Button>
        </div>
      )}
    </div>
  );
}
