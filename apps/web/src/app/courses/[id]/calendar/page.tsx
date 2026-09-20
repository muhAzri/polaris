"use client";

import Link from "next/link";
import { useParams, useRouter } from "next/navigation";
import { useEffect, useState } from "react";

import { getCourse, type Course } from "@/lib/api";
import { useAuth } from "@/lib/auth-context";
import { useCourseCapabilities } from "@/lib/use-capabilities";
import { CalendarView } from "@/components/calendar/CalendarView";
import { CourseTabs } from "@/components/course/CourseTabs";

export default function CourseCalendarPage() {
  const { id } = useParams<{ id: string }>();
  const { user, token, loading } = useAuth();
  const router = useRouter();
  const [course, setCourse] = useState<Course | null>(null);
  const { can, ready } = useCourseCapabilities(token, id);

  useEffect(() => {
    if (!loading && !user) router.push("/login");
  }, [loading, user, router]);

  useEffect(() => {
    if (!token) return;
    getCourse(token, id)
      .then(setCourse)
      .catch(() => setCourse(null));
  }, [token, id]);

  if (loading || !user || !token) return null;

  return (
    <main className="frame flex flex-1 flex-col gap-10 py-12">
      <header className="flex flex-col gap-4">
        <Link href="/courses" className="link self-start text-sm">
          ← My courses
        </Link>
        <h1 className="text-display">{course?.title ?? "Loading…"}</h1>
        <CourseTabs courseId={id} />
      </header>
      {ready ? (
        <CalendarView token={token} courseId={id} canCreate={can("calendar:manageentries")} />
      ) : (
        <p className="text-muted">Loading…</p>
      )}
    </main>
  );
}
