"use client";

import { useEffect } from "react";
import { Button } from "@/components/ui/button";

export default function Error({
  error,
  reset,
}: {
  error: Error & { digest?: string };
  reset: () => void;
}) {
  useEffect(() => {
    console.error(error);
  }, [error]);

  return (
    <div className="flex min-h-[50vh] flex-col items-center justify-center gap-2">
      <h1 className="text-2xl font-semibold">出错了</h1>
      <p className="text-muted-foreground">{error.message || "发生未知错误"}</p>
      <Button onClick={reset}>重试</Button>
    </div>
  );
}
