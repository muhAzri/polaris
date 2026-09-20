"use client";

import { useState, type FormEvent } from "react";

import { ApiError, deleteSection, updateSection, type SectionContent } from "@/lib/api";
import { ManageBar, ManageButton } from "@/components/course/ManageBar";

export function SectionHeader({
  token,
  section,
  count,
  canManage,
  onChanged,
}: {
  token: string;
  section: SectionContent;
  count: number;
  canManage: boolean;
  onChanged: () => void;
}) {
  const [editing, setEditing] = useState(false);
  const [title, setTitle] = useState(section.title);
  const [summary, setSummary] = useState(section.summary);
  const [error, setError] = useState<string | null>(null);
  const isGeneral = section.position === 0;

  async function run(action: () => Promise<unknown>) {
    setError(null);
    try {
      await action();
      setEditing(false);
      onChanged();
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Could not update the section");
    }
  }

  function save(e: FormEvent) {
    e.preventDefault();
    return run(() => updateSection(token, section.id, { title, summary }));
  }

  return (
    <div className="flex flex-col gap-3">
      {editing ? (
        <form onSubmit={save} className="flex flex-col gap-3">
          <label className="field">
            <span className="field-label">Section title</span>
            <input value={title} onChange={(e) => setTitle(e.target.value)} className="input" />
          </label>
          <label className="field">
            <span className="field-label">Summary</span>
            <textarea
              value={summary}
              onChange={(e) => setSummary(e.target.value)}
              rows={2}
              className="input"
            />
          </label>
          <div className="flex gap-2">
            <button type="submit" className="btn btn-primary btn-sm">
              Save
            </button>
            <button type="button" onClick={() => setEditing(false)} className="btn btn-quiet btn-sm">
              Cancel
            </button>
          </div>
        </form>
      ) : (
        <>
          <h2 className="border-b-2 border-ink pb-2 text-2xl">
            {section.title || "Untitled section"}
            {!section.visible ? <span className="meta ml-3">Hidden from students</span> : null}
          </h2>
          {section.summary ? <p className="prose-measure text-muted">{section.summary}</p> : null}
        </>
      )}

      {canManage && !editing ? (
        <ManageBar>
          <ManageButton onClick={() => setEditing(true)}>Edit section</ManageButton>
          <ManageButton
            onClick={() => run(() => updateSection(token, section.id, { visible: !section.visible }))}
          >
            {section.visible ? "Hide" : "Show"}
          </ManageButton>
          {!isGeneral ? (
            <>
              <ManageButton
                label="Move section up"
                disabled={section.position <= 1}
                onClick={() => run(() => updateSection(token, section.id, { position: section.position - 1 }))}
              >
                ↑
              </ManageButton>
              <ManageButton
                label="Move section down"
                disabled={section.position >= count - 1}
                onClick={() => run(() => updateSection(token, section.id, { position: section.position + 1 }))}
              >
                ↓
              </ManageButton>
              <ManageButton
                onClick={() => {
                  if (window.confirm("Delete this section and everything in it?")) {
                    run(() => deleteSection(token, section.id));
                  }
                }}
              >
                Delete section
              </ManageButton>
            </>
          ) : null}
        </ManageBar>
      ) : null}

      {error ? (
        <p role="alert" className="notice">
          {error}
        </p>
      ) : null}
    </div>
  );
}
