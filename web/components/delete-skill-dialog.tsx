"use client";

import { useState } from "react";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogClose,
} from "@/components/ui/dialog";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { skillApi } from "@/lib/api";
import { ApiError } from "@/lib/types";

interface Props {
  slug: string;
  name: string;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onDeleted: () => void;
}

export function DeleteSkillDialog({ slug, name, open, onOpenChange, onDeleted }: Props) {
  const [confirm, setConfirm] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);

  async function onSubmit(e: React.FormEvent) {
    e.preventDefault();
    setSubmitting(true);
    setError(null);
    try {
      await skillApi.delete(slug, name);
      setConfirm("");
      onOpenChange(false);
      onDeleted();
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "删除失败");
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>删除 skill · {name}</DialogTitle>
          <DialogDescription>
            将删除该 skill 的所有版本与存储对象，此操作不可恢复。
          </DialogDescription>
        </DialogHeader>
        <form onSubmit={onSubmit} className="flex flex-col gap-4">
          <div className="flex flex-col gap-2">
            <Label htmlFor="confirm-del">
              输入 skill 名称 <code className="font-mono">{name}</code> 以确认
            </Label>
            <Input
              id="confirm-del"
              value={confirm}
              onChange={(e) => setConfirm(e.target.value)}
              placeholder={name}
              className="font-mono"
            />
          </div>
          {error && <p className="text-sm text-destructive">{error}</p>}
          <DialogFooter>
            <DialogClose render={<Button variant="outline" type="button">取消</Button>} />
            <Button type="submit" variant="destructive" disabled={submitting || confirm !== name}>
              {submitting ? "删除中…" : "删除"}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
