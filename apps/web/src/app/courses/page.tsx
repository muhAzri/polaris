"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { useCallback, useEffect, useState } from "react";

import { ApiError, enrollCourse, listAvailableCourses, listCourses, type Course } from "@/lib/api";
import { useAuth } from "@/lib/auth-context";

export default function CoursesPage() {
  const { user, token, loading } = useAuth();
  const router = useRouter();
  const [courses, setCourses] = useState<Course[] | null>(null);
  const [available, setAvailable] = useState<Course[]>([]);
  const [error, setError] = useState<string | null>(null);
  const [joining, setJoining] = useState<string | null>(null);
  const [keys, setKeys] = useState<Record<string, string>>({});

  useEffect(() => {
    if (!loading && !user) router.push("/login");
  }, [loading, user, router]);

  const load = useCallback(() => {
    if (!token) return;
    listCourses(token)
      .then(setCourses)
      .catch(() => setError("Could not load courses"));
    listAvailableCourses(token)
      .then(setAvailable)
      .catch(() => setAvailable([]));
  }, [token]);

  useEffect(load, [load]);

  async function handleJoin(courseId: string) {
    if (!token) return;
    setError(null);
    setJoining(courseId);
    try {
      await enrollCourse(token, courseId, keys[courseId] ?? "");
      load();
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Could not enroll");
    } finally {
      setJoining(null);
    }
  }

  if (loading || !user) return null;

  const canCreateCourse = user.role === "teacher" || user.role === "admin";

  return (
    <main className="frame flex flex-1 flex-col gap-12 py-12">
      <section className="flex flex-col gap-8">
        <header className="flex flex-wrap items-end justify-between gap-4">
          <div className="flex flex-col gap-2">
            <h1 className="text-display">My courses</h1>
            {courses ? (
              <p className="meta">
                {courses.length} {courses.length === 1 ? "course" : "courses"}
              </p>
            ) : null}
          </div>
          {canCreateCourse ? (
            <Link href="/courses/new" className="btn btn-primary">
              New course
            </Link>
          ) : null}
        </header>

        {error ? (
          <p role="alert" className="notice">
            {error}
          </p>
        ) : null}

        {courses === null && !error ? (
          <p className="text-muted">Loading…</p>
        ) : courses?.length === 0 ? (
          <div className="border-t-2 border-ink py-10">
            <p className="font-display text-xl">You are not enrolled in any course yet.</p>
            <p className="mt-1 text-muted">
              {canCreateCourse
                ? "Create a course, then add participants to it."
                : "A teacher or admin will add you to your classes."}
            </p>
          </div>
        ) : (
          <ul className="ledger">
            {courses?.map((course) => (
              <li key={course.id}>
                <Link href={`/courses/${course.id}`} className="ledger-row">
                  <span className="flex min-w-0 flex-col gap-1">
                    <span className="ledger-title">
                      {course.title}
                      {!course.visible ? <span className="meta ml-3">Hidden</span> : null}
                    </span>
                    {course.description ? (
                      <span className="prose-measure text-sm text-muted">{course.description}</span>
                    ) : null}
                  </span>
                  <span aria-hidden className="text-muted">
                    →
                  </span>
                </Link>
              </li>
            ))}
          </ul>
        )}
      </section>

      {available.length > 0 ? (
        <section aria-labelledby="open-courses" className="flex flex-col gap-4">
          <h2 id="open-courses" className="text-2xl">
            Open for enrollment
          </h2>
          <ul className="ledger">
            {available.map((course) => (
              <li key={course.id} className="flex flex-wrap items-center justify-between gap-4 py-4">
                <span className="flex min-w-0 flex-col gap-1">
                  <span className="ledger-title">{course.title}</span>
                  {course.description ? (
                    <span className="prose-measure text-sm text-muted">{course.description}</span>
                  ) : null}
                </span>
                <span className="flex flex-wrap items-center gap-3">
                  {course.requires_key ? (
                    <input
                      type="password"
                      aria-label={`Enrolment key for ${course.title}`}
                      placeholder="Enrolment key"
                      value={keys[course.id] ?? ""}
                      onChange={(e) => setKeys({ ...keys, [course.id]: e.target.value })}
                      className="input w-44"
                    />
                  ) : null}
                  <button
                    onClick={() => handleJoin(course.id)}
                    disabled={joining === course.id}
                    aria-busy={joining === course.id}
                    className="btn btn-sm"
                  >
                    {joining === course.id ? "Joining…" : "Enroll"}
                  </button>
                </span>
              </li>
            ))}
          </ul>
        </section>
      ) : null}
    </main>
  );
}
