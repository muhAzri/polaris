"use client";

import { useState, type FormEvent } from "react";

import {
  ApiError,
  createBook,
  createFolder,
  createLabel,
  createPage,
  createResource,
  createUrl,
} from "@/lib/api";

const MODULE_TYPES = ["label", "page", "url", "resource", "folder", "book"] as const;
type ModuleType = (typeof MODULE_TYPES)[number];

export function AddModuleForm({
  token,
  sectionId,
  onAdded,
}: {
  token: string;
  sectionId: string;
  onAdded: () => void;
}) {
  const [open, setOpen] = useState(false);
  const [type, setType] = useState<ModuleType>("label");
  const [title, setTitle] = useState("");
  const [content, setContent] = useState("");
  const [url, setUrl] = useState("");
  const [description, setDescription] = useState("");
  const [file, setFile] = useState<File | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);

  function reset() {
    setTitle("");
    setContent("");
    setUrl("");
    setDescription("");
    setFile(null);
  }

  async function handleSubmit(e: FormEvent) {
    e.preventDefault();
    setError(null);
    setSubmitting(true);
    try {
      switch (type) {
        case "label":
          await createLabel(token, sectionId, content);
          break;
        case "page":
          await createPage(token, sectionId, title, content);
          break;
        case "url":
          await createUrl(token, sectionId, title, url, description);
          break;
        case "resource":
          if (!file) throw new Error("Choose a file first");
          await createResource(token, sectionId, title, file);
          break;
        case "folder":
          await createFolder(token, sectionId, title, description);
          break;
        case "book":
          await createBook(token, sectionId, title, content);
          break;
      }
      reset();
      setOpen(false);
      onAdded();
    } catch (err) {
      setError(
        err instanceof ApiError ? err.message : err instanceof Error ? err.message : "Could not add content",
      );
    } finally {
      setSubmitting(false);
    }
  }

  if (!open) {
    return (
      <button onClick={() => setOpen(true)} className="btn btn-quiet btn-sm self-start">
        Add content
      </button>
    );
  }

  return (
    <form
      onSubmit={handleSubmit}
      className="flex max-w-xl flex-col gap-4 border-t-2 border-ink bg-paper-2 p-4"
    >
      <label className="field">
        <span className="field-label">Type</span>
        <select
          value={type}
          onChange={(e) => setType(e.target.value as ModuleType)}
          className="input"
        >
          {MODULE_TYPES.map((t) => (
            <option key={t} value={t}>
              {t}
            </option>
          ))}
        </select>
      </label>

      {type !== "label" ? (
        <label className="field">
          <span className="field-label">Title</span>
          <input required value={title} onChange={(e) => setTitle(e.target.value)} className="input" />
        </label>
      ) : null}

      {type === "label" || type === "page" || type === "book" ? (
        <label className="field">
          <span className="field-label">{type === "book" ? "Intro (optional)" : "Content"}</span>
          <textarea
            required={type !== "book"}
            value={content}
            onChange={(e) => setContent(e.target.value)}
            rows={3}
            className="input"
          />
        </label>
      ) : null}

      {type === "url" ? (
        <>
          <label className="field">
            <span className="field-label">URL</span>
            <input required type="url" value={url} onChange={(e) => setUrl(e.target.value)} className="input" />
          </label>
          <label className="field">
            <span className="field-label">Description (optional)</span>
            <input value={description} onChange={(e) => setDescription(e.target.value)} className="input" />
          </label>
        </>
      ) : null}

      {type === "folder" ? (
        <label className="field">
          <span className="field-label">Description (optional)</span>
          <input value={description} onChange={(e) => setDescription(e.target.value)} className="input" />
        </label>
      ) : null}

      {type === "resource" ? (
        <label className="field">
          <span className="field-label">File</span>
          <input
            required
            type="file"
            onChange={(e) => setFile(e.target.files?.[0] ?? null)}
            className="input py-2"
          />
        </label>
      ) : null}

      {error ? (
        <p role="alert" className="notice">
          {error}
        </p>
      ) : null}

      <div className="flex gap-2">
        <button type="submit" disabled={submitting} aria-busy={submitting} className="btn btn-primary btn-sm">
          {submitting ? "Adding…" : "Add"}
        </button>
        <button type="button" onClick={() => setOpen(false)} className="btn btn-quiet btn-sm">
          Cancel
        </button>
      </div>
    </form>
  );
}
