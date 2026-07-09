"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { Button, buttonVariants } from "@/components/ui/button";
import { useUser } from "@/components/providers/user-provider";

export function Nav() {
  const { user, loading, logout } = useUser();
  const router = useRouter();

  async function onLogout() {
    await logout();
    router.push("/login");
  }

  return (
    <header className="border-b">
      <nav className="mx-auto flex h-14 max-w-5xl items-center justify-between px-4">
        <div className="flex items-center gap-6">
          <Link href="/" className="font-semibold">
            SkillHub
          </Link>
          {user && (
            <>
              <Link href="/search" className="text-sm text-muted-foreground hover:text-foreground">
                搜索
              </Link>
              <Link href="/publish" className="text-sm text-muted-foreground hover:text-foreground">
                发布
              </Link>
              <Link href="/skills" className="text-sm text-muted-foreground hover:text-foreground">
                技能管理
              </Link>
              <Link href="/teams" className="text-sm text-muted-foreground hover:text-foreground">
                我的团队
              </Link>
              <Link href="/stars" className="text-sm text-muted-foreground hover:text-foreground">
                收藏
              </Link>
            </>
          )}
        </div>
        <div className="flex items-center gap-2">
          {loading ? null : user ? (
            <>
              <span className="text-sm text-muted-foreground">{user.username}</span>
              <Button variant="outline" size="sm" onClick={onLogout}>
                登出
              </Button>
            </>
          ) : (
            <>
              <Link href="/login" className={buttonVariants({ variant: "ghost", size: "sm" })}>
                登录
              </Link>
              <Link href="/register" className={buttonVariants({ size: "sm" })}>
                注册
              </Link>
            </>
          )}
        </div>
      </nav>
    </header>
  );
}
