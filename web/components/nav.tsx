"use client";

import Link from "next/link";
import { usePathname, useRouter } from "next/navigation";
import { Boxes } from "lucide-react";
import { Button, buttonVariants } from "@/components/ui/button";
import { cn } from "@/lib/utils";
import { useUser } from "@/components/providers/user-provider";

const NAV_LINKS = [
  { href: "/search", label: "搜索" },
  { href: "/publish", label: "发布" },
  { href: "/skills", label: "技能管理" },
  { href: "/teams", label: "我的团队" },
  { href: "/stars", label: "收藏" },
];

function isActive(pathname: string, href: string) {
  if (href === "/search") return pathname === "/" || pathname === href;
  return pathname === href || pathname.startsWith(href + "/");
}

export function Nav() {
  const { user, loading, logout } = useUser();
  const router = useRouter();
  const pathname = usePathname();

  async function onLogout() {
    await logout();
    router.push("/login");
  }

  return (
    <header className="sticky top-0 z-40 border-b border-border/70 bg-background/80 backdrop-blur-md">
      <nav className="mx-auto flex h-14 max-w-5xl items-center justify-between px-4">
        <div className="flex items-center gap-6">
          <Link href="/" className="flex items-center gap-2 font-semibold">
            <span className="flex size-7 items-center justify-center rounded-lg bg-primary text-primary-foreground">
              <Boxes className="size-4" />
            </span>
            SkillHub
          </Link>
          {user && (
            <div className="flex items-center gap-1">
              {NAV_LINKS.map((l) => {
                const active = isActive(pathname, l.href);
                return (
                  <Link
                    key={l.href}
                    href={l.href}
                    className={cn(
                      "rounded-md px-2.5 py-1.5 text-sm transition-colors",
                      active
                        ? "bg-accent font-medium text-accent-foreground"
                        : "text-muted-foreground hover:bg-muted/60 hover:text-foreground",
                    )}
                  >
                    {l.label}
                  </Link>
                );
              })}
            </div>
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
