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
import { Tabs, TabsList, TabsTrigger, TabsContent } from "@/components/ui/tabs";
import { skillApi } from "@/lib/api";
import { ApiError } from "@/lib/types";
import { createTarGz } from "@/lib/tgz";

interface Props {
  slug: string;
  name: string;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onPublished: () => void;
}

export function PublishDialog({ slug, name, open, onOpenChange, onPublished }: Props) {
  const [version, setVersion] = useState("");
  const [mode, setMode] = useState<"files" | "tarball">("files");
  const [files, setFiles] = useState<File[] | null>(null);
  const [tarball, setTarball] = useState<File | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);
  const filesRef = useRef<HTMLInputElement>(null);
  const dirRef = useRef<HTMLInputElement>(null);
  const tarballRef = useRef<HTMLInputElement>(null);

  function reset() {
    setVersion("");
    setFiles(null);
    setTarball(null);
    if (filesRef.current) filesRef.current.value = "";
    if (dirRef.current) dirRef.current.value = "";
    if (tarballRef.current) tarballRef.current.value = "";
  }

  async function onSubmit(e: React.FormEvent) {
    e.preventDefault();
    let body: ArrayBuffer;
    if (mode === "files") {
      if (!files || files.length === 0) {
        setError("请选择 skill 文件或目录");
        return;
      }
      try {
        body = await createTarGz(files);
      } catch (err) {
        setError(err instanceof Error ? err.message : "打包失败");
        return;
      }
    } else {
      if (!tarball) {
        setError("请选择一个 .tar.gz 文件");
        return;
      }
      body = await tarball.arrayBuffer();
    }
    setSubmitting(true);
    setError(null);
    try {
      await skillApi.publish(slug, name, version, body);
      reset();
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
          <Tabs value={mode} onValueChange={(v) => setMode(v as "files" | "tarball")} className="gap-2">
            <TabsList>
              <TabsTrigger value="files">文件 / 目录</TabsTrigger>
              <TabsTrigger value="tarball">已打包 .tar.gz</TabsTrigger>
            </TabsList>
            <TabsContent value="files" className="flex flex-col gap-2">
              <Label htmlFor="files">Skill 文件</Label>
              <Input
                id="files"
                ref={filesRef}
                type="file"
                multiple
                onChange={(e) => setFiles(Array.from(e.target.files ?? []))}
              />
              <Label htmlFor="dir" className="text-muted-foreground">
                或选择整个目录（保留子目录结构）
              </Label>
              <Input
                id="dir"
                ref={dirRef}
                type="file"
                {...({ webkitdirectory: "" } as Record<string, string>)}
                onChange={(e) => setFiles(Array.from(e.target.files ?? []))}
              />
              <p className="text-xs text-muted-foreground">
                浏览器会就地打成 tar.gz 再上传，无需本地预打包。
              </p>
            </TabsContent>
            <TabsContent value="tarball" className="flex flex-col gap-2">
              <Label htmlFor="tarball">Tarball</Label>
              <Input
                id="tarball"
                ref={tarballRef}
                type="file"
                accept=".tar.gz,application/gzip"
                onChange={(e) => setTarball(e.target.files?.[0] ?? null)}
              />
              <p className="text-xs text-muted-foreground">已有 .tar.gz 时直接上传。</p>
            </TabsContent>
          </Tabs>
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
