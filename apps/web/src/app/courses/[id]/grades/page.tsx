"use client";

import Link from "next/link";
import { useParams, useRouter } from "next/navigation";
import { useEffect, useState } from "react";

import { getCourse, type Course } from "@/lib/api";
import { useAuth } from "@/lib/auth-context";
import { useCourseCapabilities } from "@/lib/use-capabilities";
import { CourseTabs } from "@/components/course/CourseTabs";
import { GraderView } from "@/components/grades/GraderView";
import { MyGradesView } from "@/components/grades/MyGradesView";

export default function GradesPage() {
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

      {!ready ? (
        <p className="text-muted">Loading…</p>
      ) : can("grade:viewall") ? (
        <GraderView token={token} courseId={id} canManage={can("grade:manage")} canEdit={can("grade:edit")} />
      ) : can("grade:view") ? (
        <MyGradesView token={token} courseId={id} />
      ) : (
        <p role="alert" className="notice">
          You do not have access to the gradebook in this course.
        </p>
      )}
    </main>
  );
}
