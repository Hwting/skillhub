"use client";

import { useEffect, type ReactNode } from "react";
import { useRouter } from "next/navigation";
import { useUser } from "@/components/providers/user-provider";

// Wraps protected pages. While auth state loads, show a spinner. If it resolves
// to no user, redirect to /login.
export function AuthGuard({ children }: { children: ReactNode }) {
  const { user, loading } = useUser();
  const router = useRouter();

  useEffect(() => {
    if (!loading && !user) {
      router.replace("/login");
    }
  }, [loading, user, router]);

  if (loading) {
    return (
      <div className="flex min-h-[50vh] items-center justify-center text-muted-foreground">
        加载中…
      </div>
    );
  }
  if (!user) {
    return null;
  }
  return <>{children}</>;
}
