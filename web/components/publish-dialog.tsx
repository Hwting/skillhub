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
import { PackageInput } from "@/components/package-input";
import { skillApi } from "@/lib/api";
import { ApiError } from "@/lib/types";
import { buildPackageBody, type PackageSelection } from "@/lib/tgz";

interface Props {
  slug: string;
  name: string;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onPublished: () => void;
}

export function PublishDialog({ slug, name, open, onOpenChange, onPublished }: Props) {
  const [version, setVersion] = useState("");
  const [pkg, setPkg] = useState<PackageSelection | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);
  // Bump to remount PackageInput (clears its file inputs) after a successful publish.
  const [resetKey, setResetKey] = useState(0);

  async function onSubmit(e: React.FormEvent) {
    e.preventDefault();
    if (!pkg) {
      setError("请选择上传内容");
      return;
    }
    let body: ArrayBuffer;
    try {
      body = await buildPackageBody(pkg);
    } catch (err) {
      setError(err instanceof Error ? err.message : "打包失败");
      return;
    }
    setSubmitting(true);
    setError(null);
    try {
      await skillApi.publish(slug, name, version, body);
      setVersion("");
      setPkg(null);
      setResetKey((k) => k + 1);
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
          <DialogDescription>上传 skill 内容并指定语义化版本号</DialogDescription>
        </DialogHeader>
        <form onSubmit={onSubmit} className="flex flex-col gap-4">
          <div className="flex flex-col gap-2">
            <Label htmlFor="version">版本号</Label>
            <Input
              id="version"
              placeholder="1.0.0"
              value={version}
              onChange={(e) => setVersion(e.target.value)}
              required
            />
          </div>
          <PackageInput key={resetKey} onChange={setPkg} />
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
