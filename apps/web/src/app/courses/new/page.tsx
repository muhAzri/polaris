"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { useEffect, useState, type FormEvent } from "react";

import { ApiError, createCourse, listCategories, type Category, type CourseFormat } from "@/lib/api";
import { useAuth } from "@/lib/auth-context";

export default function NewCoursePage() {
  const { user, token, loading } = useAuth();
  const router = useRouter();
  const [title, setTitle] = useState("");
  const [shortName, setShortName] = useState("");
  const [description, setDescription] = useState("");
  const [format, setFormat] = useState<CourseFormat>("topics");
  const [categoryId, setCategoryId] = useState("");
  const [categories, setCategories] = useState<Category[]>([]);
  const [error, setError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);

  useEffect(() => {
    if (!loading && !user) router.push("/login");
  }, [loading, user, router]);

  useEffect(() => {
    if (!token) return;
    listCategories(token)
      .then(setCategories)
      .catch(() => setCategories([]));
  }, [token]);

  async function handleSubmit(e: FormEvent) {
    e.preventDefault();
    if (!token) return;
    setError(null);
    setSubmitting(true);
    try {
      const course = await createCourse(token, {
        title,
        short_name: shortName.trim() || undefined,
        description,
        format,
        category_id: categoryId || undefined,
      });
      router.push(`/courses/${course.id}`);
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Could not create course");
    } finally {
      setSubmitting(false);
    }
  }

  if (loading || !user) return null;

  return (
    <main className="frame split py-12">
      <header className="flex flex-col gap-3">
        <Link href="/courses" className="link self-start text-sm">
          ← My courses
        </Link>
        <h1 className="text-display">New course</h1>
        <p className="prose-measure text-lg text-muted">
          Give it a title and a short name. Everything else can be changed later in the course settings.
        </p>
      </header>

      <form onSubmit={handleSubmit} className="flex flex-col gap-5">
        <label className="field">
          <span className="field-label">Title</span>
          <input
            required
            value={title}
            onChange={(e) => setTitle(e.target.value)}
            className="input"
          />
        </label>
        <label className="field">
          <span className="field-label">Short name</span>
          <input
            value={shortName}
            onChange={(e) => setShortName(e.target.value)}
            className="input"
            placeholder="e.g. stat-101"
          />
          <span className="field-hint">Must be unique. Leave blank to generate one from the title.</span>
        </label>
        <label className="field">
          <span className="field-label">Category</span>
          <select value={categoryId} onChange={(e) => setCategoryId(e.target.value)} className="input">
            <option value="">Default</option>
            {categories.map((c) => (
              <option key={c.id} value={c.id}>
                {c.name}
              </option>
            ))}
          </select>
        </label>
        <label className="field">
          <span className="field-label">Format</span>
          <select
            value={format}
            onChange={(e) => setFormat(e.target.value as CourseFormat)}
            className="input"
          >
            <option value="topics">Topics</option>
            <option value="weeks">Weekly</option>
          </select>
        </label>
        <label className="field">
          <span className="field-label">Description</span>
          <textarea
            value={description}
            onChange={(e) => setDescription(e.target.value)}
            rows={4}
            className="input"
          />
        </label>

        {error ? (
          <p role="alert" className="notice">
            {error}
          </p>
        ) : null}

        <button
          type="submit"
          disabled={submitting}
          aria-busy={submitting}
          className="btn btn-primary self-start"
        >
          {submitting ? "Creating…" : "Create course"}
        </button>
      </form>
    </main>
  );
}
