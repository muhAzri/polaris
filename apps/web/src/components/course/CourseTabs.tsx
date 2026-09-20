"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";

const linkClass =
  "inline-flex min-h-11 items-center border-b-2 border-transparent text-sm text-muted transition-colors hover:text-ink aria-[current=page]:border-accent aria-[current=page]:text-ink";

export function CourseTabs({ courseId }: { courseId: string }) {
  const pathname = usePathname();
  const base = `/courses/${courseId}`;
  const tabs = [
    { href: base, label: "Course" },
    { href: `${base}/grades`, label: "Grades" },
    { href: `${base}/groups`, label: "Groups" },
    { href: `${base}/calendar`, label: "Calendar" },
  ];

  return (
    <nav aria-label="Course sections" className="flex flex-wrap gap-x-6 border-b border-rule">
      {tabs.map((tab) => (
        <Link
          key={tab.href}
          href={tab.href}
          className={`${linkClass} -mb-px`}
          aria-current={pathname === tab.href ? "page" : undefined}
        >
          {tab.label}
        </Link>
      ))}
    </nav>
  );
}
