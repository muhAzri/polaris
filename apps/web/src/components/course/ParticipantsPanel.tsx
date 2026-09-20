"use client";

import { useCallback, useEffect, useState, type FormEvent } from "react";

import {
  ApiError,
  enrolUser,
  getSelfEnrolment,
  listParticipants,
  listRoles,
  setSelfEnrolment,
  unenrolUser,
  updateEnrolment,
  type Participant,
  type RoleInfo,
  type SelfEnrolment,
} from "@/lib/api";

// Roles a teacher can hand out when enrolling; site-level roles are
// managed elsewhere.
const ENROLABLE = ["student", "nonediting_teacher", "teacher"];

function SelfEnrolmentSettings({ token, courseId }: { token: string; courseId: string }) {
  const [settings, setSettings] = useState<SelfEnrolment | null>(null);
  const [saved, setSaved] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    getSelfEnrolment(token, courseId)
      .then(setSettings)
      .catch(() => setError("Could not load enrolment settings"));
  }, [token, courseId]);

  async function handleSave(e: FormEvent) {
    e.preventDefault();
    if (!settings) return;
    setError(null);
    setSaved(false);
    try {
      await setSelfEnrolment(token, courseId, settings);
      setSaved(true);
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Could not save settings");
    }
  }

  if (!settings) return error ? <p role="alert" className="notice">{error}</p> : null;

  return (
    <form onSubmit={handleSave} className="flex flex-col gap-4 border-t border-rule pt-5">
      <h3 className="font-display text-lg font-medium">Self enrolment</h3>
      <label className="flex items-start gap-3">
        <input
          type="checkbox"
          checked={settings.enabled}
          onChange={(e) => setSettings({ ...settings, enabled: e.target.checked })}
          className="mt-1 size-4 accent-accent"
        />
        <span className="flex flex-col">
          <span className="text-sm font-medium">Allow new self enrolments</span>
          <span className="field-hint">
            When on, the course appears under &ldquo;Open for enrollment&rdquo;.
          </span>
        </span>
      </label>
      <label className="field">
        <span className="field-label">Enrolment key</span>
        <input
          value={settings.key}
          onChange={(e) => setSettings({ ...settings, key: e.target.value })}
          className="input"
          autoComplete="off"
        />
        <span className="field-hint">Leave blank to let anyone join.</span>
      </label>
      <div className="flex flex-wrap gap-4">
        <label className="field w-40">
          <span className="field-label">Max users</span>
          <input
            type="number"
            min={1}
            value={settings.max_users ?? ""}
            onChange={(e) =>
              setSettings({ ...settings, max_users: e.target.value ? Number(e.target.value) : null })
            }
            className="input"
          />
        </label>
        <label className="field w-40">
          <span className="field-label">Duration (days)</span>
          <input
            type="number"
            min={1}
            value={settings.duration_days ?? ""}
            onChange={(e) =>
              setSettings({
                ...settings,
                duration_days: e.target.value ? Number(e.target.value) : null,
              })
            }
            className="input"
          />
        </label>
      </div>
      {error ? (
        <p role="alert" className="notice">
          {error}
        </p>
      ) : null}
      {saved ? <p className="notice notice-success">Saved.</p> : null}
      <button type="submit" className="btn btn-sm self-start">
        Save enrolment settings
      </button>
    </form>
  );
}

export function ParticipantsPanel({
  token,
  courseId,
  ownerId,
  canEnrol,
  canAssignRoles,
}: {
  token: string;
  courseId: string;
  ownerId: string;
  canEnrol: boolean;
  canAssignRoles: boolean;
}) {
  const [participants, setParticipants] = useState<Participant[] | null>(null);
  const [roles, setRoles] = useState<RoleInfo[]>([]);
  const [email, setEmail] = useState("");
  const [role, setRole] = useState("student");
  const [error, setError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);

  const load = useCallback(() => {
    listParticipants(token, courseId)
      .then(setParticipants)
      .catch(() => setError("Could not load participants"));
  }, [token, courseId]);

  useEffect(load, [load]);

  useEffect(() => {
    if (!canAssignRoles && !canEnrol) return;
    listRoles(token)
      .then((all) => setRoles(all.filter((r) => ENROLABLE.includes(r.name) || !r.is_system)))
      .catch(() => setRoles([]));
  }, [token, canAssignRoles, canEnrol]);

  const roleLabel = (name: string) => roles.find((r) => r.name === name)?.display_name ?? name;

  async function run(action: () => Promise<unknown>, failure: string) {
    setError(null);
    try {
      await action();
      load();
    } catch (err) {
      setError(err instanceof ApiError ? err.message : failure);
    }
  }

  async function handleAdd(e: FormEvent) {
    e.preventDefault();
    setSubmitting(true);
    await run(async () => {
      await enrolUser(token, courseId, email.trim(), role);
      setEmail("");
    }, "Could not add participant");
    setSubmitting(false);
  }

  return (
    <section aria-labelledby="participants" className="flex flex-col gap-6">
      <h2 id="participants" className="border-b-2 border-ink pb-2 text-2xl">
        Participants
      </h2>

      <div className="split">
        <div className="flex flex-col gap-6">
          {canEnrol ? (
            <>
              <form onSubmit={handleAdd} className="flex flex-wrap items-end gap-3">
                <label className="field min-w-0 flex-1 basis-56">
                  <span className="field-label">Add by email</span>
                  <input
                    type="email"
                    required
                    value={email}
                    onChange={(e) => setEmail(e.target.value)}
                    className="input"
                  />
                </label>
                <label className="field">
                  <span className="field-label">Role</span>
                  <select value={role} onChange={(e) => setRole(e.target.value)} className="input">
                    {(roles.length ? roles : ENROLABLE.map((name) => ({ name, display_name: name }))).map(
                      (r) => (
                        <option key={r.name} value={r.name}>
                          {r.display_name}
                        </option>
                      ),
                    )}
                  </select>
                </label>
                <button
                  type="submit"
                  disabled={submitting}
                  aria-busy={submitting}
                  className="btn btn-primary"
                >
                  {submitting ? "Adding…" : "Add"}
                </button>
              </form>
              <SelfEnrolmentSettings token={token} courseId={courseId} />
            </>
          ) : null}

          {error ? (
            <p role="alert" className="notice">
              {error}
            </p>
          ) : null}
        </div>

        {participants === null ? (
          <p className="text-muted">Loading…</p>
        ) : (
          <ul className="ledger">
            {participants.map((p) => (
              <li
                key={p.user_id}
                className="flex flex-wrap items-center justify-between gap-x-4 gap-y-1 py-3"
              >
                <span className="flex min-w-0 flex-col">
                  <span>
                    {p.name}
                    {p.status === "suspended" ? <span className="meta ml-3">Suspended</span> : null}
                  </span>
                  <span className="meta break-all normal-case">
                    {p.email} · {p.roles.map(roleLabel).join(", ") || "no role"}
                    {p.method === "self" ? " · self-enrolled" : ""}
                    {p.time_end ? ` · until ${new Date(p.time_end).toLocaleDateString()}` : ""}
                  </span>
                </span>
                {p.user_id === ownerId ? (
                  <span className="meta">Owner</span>
                ) : canEnrol ? (
                  <span className="flex gap-2">
                    <button
                      onClick={() =>
                        run(
                          () =>
                            updateEnrolment(token, courseId, p.user_id, {
                              status: p.status === "active" ? "suspended" : "active",
                            }),
                          "Could not change enrolment status",
                        )
                      }
                      className="btn btn-quiet btn-sm"
                    >
                      {p.status === "active" ? "Suspend" : "Reactivate"}
                    </button>
                    <button
                      onClick={() =>
                        run(() => unenrolUser(token, courseId, p.user_id), "Could not remove participant")
                      }
                      className="btn btn-quiet btn-sm"
                    >
                      Remove
                    </button>
                  </span>
                ) : null}
              </li>
            ))}
          </ul>
        )}
      </div>
    </section>
  );
}
