"use client";

import { useState, type FormEvent } from "react";

import { addFolderFile, ApiError } from "@/lib/api";

export function AddFolderFileForm({
  token,
  moduleId,
  onAdded,
}: {
  token: string;
  moduleId: string;
  onAdded: () => void;
}) {
  const [file, setFile] = useState<File | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);

  async function handleSubmit(e: FormEvent) {
    e.preventDefault();
    if (!file) return;
    setError(null);
    setSubmitting(true);
    try {
      await addFolderFile(token, moduleId, file);
      setFile(null);
      onAdded();
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Could not add file");
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <form onSubmit={handleSubmit} className="flex flex-col gap-3 border-t border-rule pt-4">
      <div className="flex flex-wrap items-center gap-3">
        <input
          type="file"
          aria-label="File to add"
          onChange={(e) => setFile(e.target.files?.[0] ?? null)}
          className="input max-w-xs py-2 text-sm"
        />
        <button
          type="submit"
          disabled={submitting || !file}
          aria-busy={submitting}
          className="btn btn-quiet btn-sm"
        >
          {submitting ? "Uploading…" : "Add file"}
        </button>
      </div>
      {error ? (
        <p role="alert" className="notice">
          {error}
        </p>
      ) : null}
    </form>
  );
}
