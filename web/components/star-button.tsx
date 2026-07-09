"use client";

import { useState } from "react";
import { Button } from "@/components/ui/button";
import { skillApi } from "@/lib/api";
import { ApiError } from "@/lib/types";

interface Props {
  slug: string;
  name: string;
  starred: boolean;
  onToggled?: (starred: boolean) => void;
}

export function StarButton({ slug, name, starred, onToggled }: Props) {
  const [active, setActive] = useState(starred);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  async function toggle() {
    setBusy(true);
    const next = !active;
    setActive(next); // optimistic
    try {
      if (next) {
        await skillApi.star(slug, name);
      } else {
        await skillApi.unstar(slug, name);
      }
      onToggled?.(next);
    } catch (err) {
      setActive(!next); // revert
      setError(err instanceof ApiError ? err.message : "操作失败");
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="flex items-center gap-2">
      <Button variant={active ? "default" : "outline"} size="sm" onClick={toggle} disabled={busy}>
        {active ? "★ 已收藏" : "☆ 收藏"}
      </Button>
      {error && <span className="text-sm text-destructive">{error}</span>}
    </div>
  );
}
