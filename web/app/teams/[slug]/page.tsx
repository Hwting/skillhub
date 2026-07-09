"use client";

import { useEffect, useState } from "react";
import { useParams } from "next/navigation";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { AuthGuard } from "@/components/auth-guard";
import { useUser } from "@/components/providers/user-provider";
import { teamApi } from "@/lib/api";
import { ApiError, type Team } from "@/lib/types";
import { TeamSkillsTab } from "@/components/team-skills-tab";
import { TeamMembersTab } from "@/components/team-members-tab";
import { TeamSettingsTab } from "@/components/team-settings-tab";

function TeamDetailBody() {
  const params = useParams<{ slug: string }>();
  const slug = params.slug;
  const { user } = useUser();
  const [team, setTeam] = useState<Team | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  function load() {
    setLoading(true);
    teamApi
      .get(slug)
      .then(setTeam)
      .catch((err) => setError(err instanceof ApiError ? err.message : "加载失败"))
      .finally(() => setLoading(false));
  }

  useEffect(load, [slug]);

  if (loading) return <p className="mx-auto max-w-3xl px-4 py-8 text-muted-foreground">加载中…</p>;
  if (error) return <p className="mx-auto max-w-3xl px-4 py-8 text-destructive">{error}</p>;
  if (!team) return null;

  const isOwner = !!user && team.owner_user_id === user.id;

  return (
    <div className="mx-auto max-w-3xl px-4 py-8">
      <h1 className="mb-1 text-xl font-semibold">{team.name}</h1>
      <p className="mb-6 text-sm text-muted-foreground">{team.slug}</p>
      <Tabs defaultValue="skills">
        <TabsList>
          <TabsTrigger value="skills">Skills</TabsTrigger>
          <TabsTrigger value="members">成员</TabsTrigger>
          <TabsTrigger value="settings">设置</TabsTrigger>
        </TabsList>
        <TabsContent value="skills">
          <TeamSkillsTab slug={slug} />
        </TabsContent>
        <TabsContent value="members">
          <TeamMembersTab slug={slug} isOwner={isOwner} currentUserId={user?.id ?? ""} onTeamChanged={load} />
        </TabsContent>
        <TabsContent value="settings">
          <TeamSettingsTab team={team} isOwner={isOwner} onTeamChanged={load} />
        </TabsContent>
      </Tabs>
    </div>
  );
}

export default function TeamDetailPage() {
  return (
    <AuthGuard>
      <TeamDetailBody />
    </AuthGuard>
  );
}
