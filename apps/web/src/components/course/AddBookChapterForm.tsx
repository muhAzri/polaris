"use client";

import { useState, type FormEvent } from "react";

import { addBookChapter, ApiError } from "@/lib/api";

export function AddBookChapterForm({
  token,
  moduleId,
  onAdded,
}: {
  token: string;
  moduleId: string;
  onAdded: () => void;
}) {
  const [title, setTitle] = useState("");
  const [content, setContent] = useState("");
  const [subchapter, setSubchapter] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);

  async function handleSubmit(e: FormEvent) {
    e.preventDefault();
    setError(null);
    setSubmitting(true);
    try {
      await addBookChapter(token, moduleId, title, content, subchapter);
      setTitle("");
      setContent("");
      setSubchapter(false);
      onAdded();
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Could not add chapter");
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <form onSubmit={handleSubmit} className="flex max-w-xl flex-col gap-3 border-t border-rule pt-4">
      <label className="field">
        <span className="field-label">Chapter title</span>
        <input required value={title} onChange={(e) => setTitle(e.target.value)} className="input" />
      </label>
      <label className="field">
        <span className="field-label">Chapter content</span>
        <textarea
          required
          value={content}
          onChange={(e) => setContent(e.target.value)}
          rows={3}
          className="input"
        />
      </label>
      <label className="flex items-center gap-3">
        <input
          type="checkbox"
          checked={subchapter}
          onChange={(e) => setSubchapter(e.target.checked)}
          className="size-4 accent-accent"
        />
        <span className="text-sm">Subchapter of the previous chapter</span>
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
        className="btn btn-quiet btn-sm self-start"
      >
        {submitting ? "Adding…" : "Add chapter"}
      </button>
    </form>
  );
}
