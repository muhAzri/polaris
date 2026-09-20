"use client";

import Link from "next/link";
import { usePathname, useRouter } from "next/navigation";

import { useAuth } from "@/lib/auth-context";

const linkClass =
  "inline-flex min-h-11 items-center border-b-2 border-transparent text-sm text-muted transition-colors hover:text-ink aria-[current=page]:border-accent aria-[current=page]:text-ink";

export function Nav() {
  const { user, loading, logout } = useAuth();
  const router = useRouter();
  const pathname = usePathname();

  function handleLogout() {
    logout();
    router.push("/");
  }

  return (
    <header>
      {/* account strip */}
      <div className="border-b border-rule bg-paper-2">
        <div className="frame flex min-h-11 items-center justify-between gap-4">
          <span className="meta hidden sm:inline">Open-source LMS</span>
          <div className="ml-auto flex items-center gap-4">
            {loading ? null : user ? (
              <>
                <span className="text-sm">
                  {user.name} <span className="meta ml-1">{user.role}</span>
                </span>
                <button
                  onClick={handleLogout}
                  className="inline-flex min-h-11 items-center text-sm text-muted transition-colors hover:text-ink"
                >
                  Log out
                </button>
              </>
            ) : (
              <>
                <Link
                  href="/login"
                  className={linkClass}
                  aria-current={pathname === "/login" ? "page" : undefined}
                >
                  Log in
                </Link>
                <Link
                  href="/register"
                  className={linkClass}
                  aria-current={pathname === "/register" ? "page" : undefined}
                >
                  Sign up
                </Link>
              </>
            )}
          </div>
        </div>
      </div>

      {/* masthead */}
      <div className="border-b-4 border-double border-ink">
        <nav className="frame flex flex-wrap items-end gap-x-8 gap-y-1 pt-4">
          <Link href="/" className="pb-2 font-display text-4xl font-semibold leading-none tracking-tight">
            Polaris
          </Link>
          {user ? (
            <Link
              href="/courses"
              className={`${linkClass} -mb-px`}
              aria-current={pathname.startsWith("/courses") ? "page" : undefined}
            >
              My courses
            </Link>
          ) : null}
          {user ? (
            <Link
              href="/calendar"
              className={`${linkClass} -mb-px`}
              aria-current={pathname.startsWith("/calendar") ? "page" : undefined}
            >
              Calendar
            </Link>
          ) : null}
        </nav>
      </div>
    </header>
  );
}
