"use client";

import { useRef, useState } from "react";
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
  onPublished: () => void;
}

export function PublishDialog({ slug, name, open, onOpenChange, onPublished }: Props) {
  const [version, setVersion] = useState("");
  const [file, setFile] = useState<File | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);
  const inputRef = useRef<HTMLInputElement>(null);

  async function onSubmit(e: React.FormEvent) {
    e.preventDefault();
    if (!file) {
      setError("请选择一个 .tar.gz 文件");
      return;
    }
    setSubmitting(true);
    setError(null);
    try {
      const buf = await file.arrayBuffer();
      await skillApi.publish(slug, name, version, buf);
      setVersion("");
      setFile(null);
      if (inputRef.current) inputRef.current.value = "";
      onOpenChange(false);
      onPublished();
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "发布失败");
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>发布新版本 · {name}</DialogTitle>
          <DialogDescription>上传一个 tarball 并指定语义化版本号</DialogDescription>
        </DialogHeader>
        <form onSubmit={onSubmit} className="flex flex-col gap-4">
          <div className="flex flex-col gap-2">
            <Label htmlFor="version">版本号</Label>
            <Input id="version" placeholder="1.0.0" value={version} onChange={(e) => setVersion(e.target.value)} required />
          </div>
          <div className="flex flex-col gap-2">
            <Label htmlFor="file">Tarball</Label>
            <Input
              id="file"
              ref={inputRef}
              type="file"
              accept=".tar.gz,application/gzip"
              onChange={(e) => setFile(e.target.files?.[0] ?? null)}
              required
            />
          </div>
          {error && <p className="text-sm text-destructive">{error}</p>}
          <DialogFooter>
            <DialogClose render={<Button variant="outline" type="button">取消</Button>} />
            <Button type="submit" disabled={submitting}>
              {submitting ? "发布中…" : "发布"}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
