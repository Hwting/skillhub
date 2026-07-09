"use client";

import { useEffect, useState } from "react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { Badge } from "@/components/ui/badge";
import { teamApi } from "@/lib/api";
import { ApiError, type Member } from "@/lib/types";

interface Props {
  slug: string;
  isOwner: boolean;
  currentUserId: string;
  onTeamChanged: () => void;
}

export function TeamMembersTab({ slug, isOwner, currentUserId, onTeamChanged }: Props) {
  const [members, setMembers] = useState<Member[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [newUid, setNewUid] = useState("");
  const [newRole, setNewRole] = useState("member");
  const [transferUid, setTransferUid] = useState("");

  function load() {
    setLoading(true);
    teamApi
      .listMembers(slug)
      .then((res) => setMembers(res.items))
      .catch((err) => setError(err instanceof ApiError ? err.message : "加载失败"))
      .finally(() => setLoading(false));
  }
  useEffect(load, [slug]);

  // current user is admin if they're an admin member (owner handled via isOwner)
  const isAdmin = members.some((m) => m.user_id === currentUserId && m.role === "admin");
  const canMutate = isOwner || isAdmin; // add/remove member
  const sorted = [...members].sort((a, b) => (a.role === "owner" ? -1 : 1) - (b.role === "owner" ? -1 : 1));

  async function add(e: React.FormEvent) {
    e.preventDefault();
    try {
      await teamApi.addMember(slug, { user_id: newUid, role: newRole });
      setNewUid("");
      load();
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "添加失败");
    }
  }

  async function changeRole(uid: string, role: string) {
    try {
      await teamApi.patchMember(slug, uid, { role });
      load();
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "修改失败");
    }
  }

  async function remove(uid: string) {
    try {
      await teamApi.removeMember(slug, uid);
      load();
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "移除失败");
    }
  }

  async function transfer(e: React.FormEvent) {
    e.preventDefault();
    try {
      await teamApi.transfer(slug, { new_owner_id: transferUid });
      setTransferUid("");
      onTeamChanged();
      load();
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "转移失败");
    }
  }

  if (loading) return <p className="text-muted-foreground">加载中…</p>;
  return (
    <div className="flex flex-col gap-4">
      {error && <p className="text-sm text-destructive">{error}</p>}
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead>用户 ID</TableHead>
            <TableHead>角色</TableHead>
            <TableHead>操作</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {sorted.map((m) => (
            <TableRow key={m.user_id}>
              <TableCell className="font-mono text-xs">{m.user_id}</TableCell>
              <TableCell>
                <Badge variant={m.role === "owner" ? "default" : "secondary"}>{m.role}</Badge>
              </TableCell>
              <TableCell>
                {m.role !== "owner" && canMutate && (
                  <div className="flex gap-2">
                    {isOwner && (
                      <Button size="sm" variant="outline" onClick={() => changeRole(m.user_id, m.role === "admin" ? "member" : "admin")}>
                        设为{m.role === "admin" ? "成员" : "管理员"}
                      </Button>
                    )}
                    <Button size="sm" variant="destructive" onClick={() => remove(m.user_id)}>
                      移除
                    </Button>
                  </div>
                )}
              </TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>

      {canMutate && (
        <form onSubmit={add} className="flex items-end gap-2 rounded-lg border p-4">
          <div className="flex flex-col gap-1">
            <Label htmlFor="new-uid">用户 ID</Label>
            <Input id="new-uid" value={newUid} onChange={(e) => setNewUid(e.target.value)} required className="w-64 font-mono text-xs" />
          </div>
          <div className="flex flex-col gap-1">
            <Label htmlFor="new-role">角色</Label>
            <select id="new-role" value={newRole} onChange={(e) => setNewRole(e.target.value)} className="h-8 rounded-lg border px-2 text-sm">
              <option value="member">member</option>
              <option value="admin">admin</option>
            </select>
          </div>
          <Button type="submit" size="sm">添加成员</Button>
        </form>
      )}

      {isOwner && (
        <form onSubmit={transfer} className="flex items-end gap-2 rounded-lg border p-4">
          <div className="flex flex-col gap-1">
            <Label htmlFor="transfer-uid">转移所有权给（用户 ID）</Label>
            <Input id="transfer-uid" value={transferUid} onChange={(e) => setTransferUid(e.target.value)} required className="w-64 font-mono text-xs" />
          </div>
          <Button type="submit" size="sm" variant="outline">转移所有权</Button>
        </form>
      )}
    </div>
  );
}
