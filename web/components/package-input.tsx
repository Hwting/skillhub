"use client";

import { useRef, useState } from "react";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Tabs, TabsList, TabsTrigger, TabsContent } from "@/components/ui/tabs";
import type { PackageMode, PackageSelection } from "@/lib/tgz";

// Reusable upload picker shared by the /publish page and the publish dialog.
// Lifts the raw selection up via onChange; the parent builds the request body
// with buildPackageBody at submit time.
export function PackageInput({ onChange }: { onChange: (sel: PackageSelection) => void }) {
  const [mode, setMode] = useState<PackageMode>("files");
  const [files, setFiles] = useState<File[] | null>(null);
  const [tarball, setTarball] = useState<File | null>(null);
  const filesRef = useRef<HTMLInputElement>(null);
  const dirRef = useRef<HTMLInputElement>(null);
  const tarballRef = useRef<HTMLInputElement>(null);

  function changeMode(v: string) {
    const m = v as PackageMode;
    setMode(m);
    onChange({ mode: m, files, tarball });
  }

  function changeFiles(list: File[] | null) {
    setFiles(list);
    onChange({ mode, files: list, tarball });
  }

  function changeTarball(f: File | null) {
    setTarball(f);
    onChange({ mode, files, tarball: f });
  }

  return (
    <Tabs value={mode} onValueChange={changeMode} className="gap-2">
      <TabsList>
        <TabsTrigger value="files">文件 / 目录</TabsTrigger>
        <TabsTrigger value="tarball">已打包 .tar.gz</TabsTrigger>
      </TabsList>
      <TabsContent value="files" className="flex flex-col gap-2">
        <Label htmlFor="pkg-files">Skill 文件</Label>
        <Input
          id="pkg-files"
          ref={filesRef}
          type="file"
          multiple
          onChange={(e) => changeFiles(Array.from(e.target.files ?? []))}
        />
        <Label htmlFor="pkg-dir" className="text-muted-foreground">
          或选择整个目录（保留子目录结构）
        </Label>
        <Input
          id="pkg-dir"
          ref={dirRef}
          type="file"
          {...({ webkitdirectory: "" } as Record<string, string>)}
          onChange={(e) => changeFiles(Array.from(e.target.files ?? []))}
        />
        <p className="text-xs text-muted-foreground">
          浏览器会就地打成 tar.gz 再上传，无需本地预打包。
        </p>
      </TabsContent>
      <TabsContent value="tarball" className="flex flex-col gap-2">
        <Label htmlFor="pkg-tarball">Tarball</Label>
        <Input
          id="pkg-tarball"
          ref={tarballRef}
          type="file"
          accept=".tar.gz,application/gzip"
          onChange={(e) => changeTarball(e.target.files?.[0] ?? null)}
        />
        <p className="text-xs text-muted-foreground">已有 .tar.gz 时直接上传。</p>
      </TabsContent>
    </Tabs>
  );
}
