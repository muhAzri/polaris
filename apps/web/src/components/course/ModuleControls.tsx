"use client";

import { useState, type FormEvent } from "react";

import {
  ApiError,
  deleteModule,
  updateModule,
  type ModuleContent,
  type SectionContent,
} from "@/lib/api";
import { ManageBar, ManageButton } from "@/components/course/ManageBar";

function toLocalInput(iso: string | null) {
  if (!iso) return "";
  const d = new Date(iso);
  const pad = (n: number) => String(n).padStart(2, "0");
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`;
}

export function ModuleControls({
  token,
  module,
  sections,
  index,
  count,
  onChanged,
}: {
  token: string;
  module: ModuleContent;
  sections: SectionContent[];
  index: number;
  count: number;
  onChanged: () => void;
}) {
  const [error, setError] = useState<string | null>(null);
  const [showWindow, setShowWindow] = useState(false);
  const [from, setFrom] = useState(toLocalInput(module.available_from));
  const [until, setUntil] = useState(toLocalInput(module.available_until));

  async function run(action: () => Promise<unknown>) {
    setError(null);
    try {
      await action();
      onChanged();
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Could not update the module");
    }
  }

  function saveWindow(e: FormEvent) {
    e.preventDefault();
    if (!from && !until) {
      return run(() => updateModule(token, module.id, { clear_availability: true }));
    }
    return run(() =>
      updateModule(token, module.id, {
        available_from: from ? new Date(from).toISOString() : undefined,
        available_until: until ? new Date(until).toISOString() : undefined,
      }),
    );
  }

  return (
    <div className="flex flex-col gap-2">
      <ManageBar>
        <ManageButton
          onClick={() => run(() => updateModule(token, module.id, { visible: !module.visible }))}
        >
          {module.visible ? "Hide" : "Show"}
        </ManageButton>
        <ManageButton
          label="Move up"
          disabled={index === 0}
          onClick={() => run(() => updateModule(token, module.id, { position: index - 1 }))}
        >
          ↑
        </ManageButton>
        <ManageButton
          label="Move down"
          disabled={index === count - 1}
          onClick={() => run(() => updateModule(token, module.id, { position: index + 1 }))}
        >
          ↓
        </ManageButton>
        <select
          aria-label="Move to section"
          value=""
          onChange={(e) =>
            e.target.value &&
            run(() => updateModule(token, module.id, { section_id: e.target.value }))
          }
          className="input w-auto py-1 text-sm"
        >
          <option value="">Move to…</option>
          {sections.map((s) => (
            <option key={s.id} value={s.id}>
              {s.title || "Untitled section"}
            </option>
          ))}
        </select>
        <ManageButton onClick={() => setShowWindow(!showWindow)}>Availability</ManageButton>
        <ManageButton
          onClick={() => {
            if (window.confirm("Delete this activity? This cannot be undone.")) {
              run(() => deleteModule(token, module.id));
            }
          }}
        >
          Delete
        </ManageButton>
      </ManageBar>

      {showWindow ? (
        <form onSubmit={saveWindow} className="flex flex-wrap items-end gap-3">
          <label className="field">
            <span className="field-label">Opens</span>
            <input
              type="datetime-local"
              value={from}
              onChange={(e) => setFrom(e.target.value)}
              className="input"
            />
          </label>
          <label className="field">
            <span className="field-label">Closes</span>
            <input
              type="datetime-local"
              value={until}
              onChange={(e) => setUntil(e.target.value)}
              className="input"
            />
          </label>
          <button type="submit" className="btn btn-sm">
            Save
          </button>
          <span className="field-hint">Leave both empty to remove the restriction.</span>
        </form>
      ) : null}

      {error ? (
        <p role="alert" className="notice">
          {error}
        </p>
      ) : null}
    </div>
  );
}
