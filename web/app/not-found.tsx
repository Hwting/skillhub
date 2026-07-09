import Link from "next/link";

export default function NotFound() {
  return (
    <div className="flex min-h-[50vh] flex-col items-center justify-center gap-2">
      <h1 className="text-2xl font-semibold">404</h1>
      <p className="text-muted-foreground">页面不存在</p>
      <Link href="/" className="text-primary underline">
        返回首页
      </Link>
    </div>
  );
}
