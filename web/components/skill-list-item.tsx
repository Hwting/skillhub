import Link from "next/link";
import { Star } from "lucide-react";
import { Badge } from "@/components/ui/badge";
import type { SearchResult } from "@/lib/types";

function formatSize(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KiB`;
  return `${(bytes / 1024 / 1024).toFixed(1)} MiB`;
}

export function SkillListItem({ skill }: { skill: SearchResult }) {
  return (
    <Link
      href={`/teams/${skill.team_slug}/skills/${skill.name}`}
      className="group flex items-center justify-between rounded-xl border bg-card p-4 transition-all hover:border-primary/30 hover:shadow-pop"
    >
      <div className="flex flex-col gap-1.5">
        <div className="flex items-center gap-2">
          <span className="font-medium">{skill.name}</span>
          <Badge variant="secondary">{skill.team_slug}</Badge>
        </div>
        {skill.latest_version ? (
          <span className="text-sm text-muted-foreground">
            v{skill.latest_version.version} · {formatSize(skill.latest_version.size)}
          </span>
        ) : (
          <span className="text-sm text-muted-foreground">无版本</span>
        )}
      </div>
      <Star className="size-4 text-muted-foreground/40 transition-colors group-hover:text-primary/60" />
    </Link>
  );
}
