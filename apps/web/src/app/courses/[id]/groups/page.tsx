"use client";

import Link from "next/link";
import { useParams, useRouter } from "next/navigation";
import { useCallback, useEffect, useState, type FormEvent } from "react";

import {
  ApiError,
  addGroupMember,
  addGroupToGrouping,
  autoCreateGroups,
  createGroup,
  createGrouping,
  deleteGroup,
  deleteGrouping,
  getCourse,
  listGroupings,
  listGroups,
  listParticipants,
  removeGroupFromGrouping,
  removeGroupMember,
  type Course,
  type Group,
  type Grouping,
  type Participant,
} from "@/lib/api";
import { useAuth } from "@/lib/auth-context";
import { useCourseCapabilities } from "@/lib/use-capabilities";
import { CourseTabs } from "@/components/course/CourseTabs";

export default function GroupsPage() {
  const { id } = useParams<{ id: string }>();
  const { user, token, loading } = useAuth();
  const router = useRouter();
  const { can, ready } = useCourseCapabilities(token, id);
  const canManage = can("group:manage");

  const [course, setCourse] = useState<Course | null>(null);
  const [groups, setGroups] = useState<Group[] | null>(null);
  const [groupings, setGroupings] = useState<Grouping[]>([]);
  const [participants, setParticipants] = useState<Participant[]>([]);
  const [error, setError] = useState<string | null>(null);
  const [name, setName] = useState("");
  const [autoCount, setAutoCount] = useState("2");
  const [autoPrefix, setAutoPrefix] = useState("Team");
  const [groupingName, setGroupingName] = useState("");

  useEffect(() => {
    if (!loading && !user) router.push("/login");
  }, [loading, user, router]);

  const load = useCallback(() => {
    if (!token) return;
    listGroups(token, id)
      .then(setGroups)
      .catch(() => setError("Could not load groups"));
    listGroupings(token, id)
      .then(setGroupings)
      .catch(() => setGroupings([]));
  }, [token, id]);

  useEffect(load, [load]);

  useEffect(() => {
    if (!token) return;
    getCourse(token, id)
      .then(setCourse)
      .catch(() => setCourse(null));
  }, [token, id]);

  useEffect(() => {
    if (!token || !ready || !can("course:viewparticipants")) return;
    listParticipants(token, id)
      .then(setParticipants)
      .catch(() => setParticipants([]));
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [token, ready, id]);

  async function run(action: () => Promise<unknown>, failure: string) {
    setError(null);
    try {
      await action();
      load();
    } catch (err) {
      setError(err instanceof ApiError ? err.message : failure);
    }
  }

  function handleCreate(e: FormEvent) {
    e.preventDefault();
    if (!token) return;
    return run(async () => {
      await createGroup(token, id, name.trim());
      setName("");
    }, "Could not create the group");
  }

  function handleAuto(e: FormEvent) {
    e.preventDefault();
    if (!token) return;
    return run(
      () => autoCreateGroups(token, id, { count: Number(autoCount), prefix: autoPrefix.trim() }),
      "Could not create groups",
    );
  }

  function handleCreateGrouping(e: FormEvent) {
    e.preventDefault();
    if (!token) return;
    return run(async () => {
      await createGrouping(token, id, groupingName.trim());
      setGroupingName("");
    }, "Could not create the grouping");
  }

  if (loading || !user || !token) return null;

  const candidates = (group: Group) =>
    participants.filter(
      (p) => p.roles.includes("student") && !group.members.some((m) => m.user_id === p.user_id),
    );

  return (
    <main className="frame flex flex-1 flex-col gap-10 py-12">
      <header className="flex flex-col gap-4">
        <Link href="/courses" className="link self-start text-sm">
          ← My courses
        </Link>
        <h1 className="text-display">{course?.title ?? "Loading…"}</h1>
        <CourseTabs courseId={id} />
      </header>

      {error ? (
        <p role="alert" className="notice">
          {error}
        </p>
      ) : null}

      <section aria-labelledby="groups" className="flex flex-col gap-6">
        <h2 id="groups" className="border-b-2 border-ink pb-2 text-2xl">
          Groups
        </h2>

        {groups === null ? (
          <p className="text-muted">Loading…</p>
        ) : groups.length === 0 ? (
          <p className="text-muted">This course has no groups yet.</p>
        ) : (
          <ul className="ledger">
            {groups.map((group) => (
              <li key={group.id} className="flex flex-col gap-3 py-4">
                <div className="flex flex-wrap items-center justify-between gap-3">
                  <span className="ledger-title">
                    {group.name} <span className="meta ml-2">{group.members.length} members</span>
                  </span>
                  {canManage ? (
                    <button
                      type="button"
                      className="btn btn-quiet btn-sm"
                      onClick={() => {
                        if (window.confirm(`Delete group "${group.name}"?`)) {
                          run(() => deleteGroup(token, group.id), "Could not delete the group");
                        }
                      }}
                    >
                      Delete
                    </button>
                  ) : null}
                </div>

                {group.members.length > 0 ? (
                  <ul className="flex flex-wrap gap-x-4 gap-y-1 text-sm">
                    {group.members.map((m) => (
                      <li key={m.user_id} className="flex items-center gap-2">
                        {m.name}
                        {canManage ? (
                          <button
                            type="button"
                            aria-label={`Remove ${m.name} from ${group.name}`}
                            className="link text-xs"
                            onClick={() =>
                              run(() => removeGroupMember(token, group.id, m.user_id), "Could not remove the member")
                            }
                          >
                            remove
                          </button>
                        ) : null}
                      </li>
                    ))}
                  </ul>
                ) : null}

                {canManage && candidates(group).length > 0 ? (
                  <select
                    aria-label={`Add a member to ${group.name}`}
                    value=""
                    onChange={(e) =>
                      e.target.value &&
                      run(() => addGroupMember(token, group.id, e.target.value), "Could not add the member")
                    }
                    className="input w-auto self-start py-1 text-sm"
                  >
                    <option value="">Add member…</option>
                    {candidates(group).map((p) => (
                      <option key={p.user_id} value={p.user_id}>
                        {p.name}
                      </option>
                    ))}
                  </select>
                ) : null}
              </li>
            ))}
          </ul>
        )}

        {canManage ? (
          <div className="split">
            <form onSubmit={handleCreate} className="flex flex-wrap items-end gap-3">
              <label className="field min-w-0 flex-1 basis-48">
                <span className="field-label">New group</span>
                <input required value={name} onChange={(e) => setName(e.target.value)} className="input" />
              </label>
              <button type="submit" className="btn">
                Create group
              </button>
            </form>
            <form onSubmit={handleAuto} className="flex flex-wrap items-end gap-3">
              <label className="field w-28">
                <span className="field-label">Groups</span>
                <input
                  type="number"
                  min={1}
                  value={autoCount}
                  onChange={(e) => setAutoCount(e.target.value)}
                  className="input"
                />
              </label>
              <label className="field min-w-0 flex-1 basis-36">
                <span className="field-label">Name prefix</span>
                <input value={autoPrefix} onChange={(e) => setAutoPrefix(e.target.value)} className="input" />
              </label>
              <button type="submit" className="btn">
                Split students randomly
              </button>
            </form>
          </div>
        ) : null}
      </section>

      <section aria-labelledby="groupings" className="flex flex-col gap-6">
        <h2 id="groupings" className="border-b-2 border-ink pb-2 text-2xl">
          Groupings
        </h2>
        <p className="prose-measure text-muted">
          A grouping bundles groups so an activity can be limited to them.
        </p>

        {groupings.length === 0 ? (
          <p className="text-muted">No groupings yet.</p>
        ) : (
          <ul className="ledger">
            {groupings.map((grouping) => (
              <li key={grouping.id} className="flex flex-col gap-3 py-4">
                <div className="flex flex-wrap items-center justify-between gap-3">
                  <span className="ledger-title">{grouping.name}</span>
                  {canManage ? (
                    <button
                      type="button"
                      className="btn btn-quiet btn-sm"
                      onClick={() => run(() => deleteGrouping(token, grouping.id), "Could not delete the grouping")}
                    >
                      Delete
                    </button>
                  ) : null}
                </div>
                <ul className="flex flex-wrap gap-x-5 gap-y-2">
                  {(groups ?? []).map((g) => {
                    const inside = grouping.group_ids.includes(g.id);
                    return (
                      <li key={g.id}>
                        <label className="flex items-center gap-2 text-sm">
                          <input
                            type="checkbox"
                            checked={inside}
                            disabled={!canManage}
                            onChange={() =>
                              run(
                                () =>
                                  inside
                                    ? removeGroupFromGrouping(token, grouping.id, g.id)
                                    : addGroupToGrouping(token, grouping.id, g.id),
                                "Could not update the grouping",
                              )
                            }
                            className="size-4 accent-accent"
                          />
                          {g.name}
                        </label>
                      </li>
                    );
                  })}
                </ul>
              </li>
            ))}
          </ul>
        )}

        {canManage ? (
          <form onSubmit={handleCreateGrouping} className="flex flex-wrap items-end gap-3">
            <label className="field min-w-0 flex-1 basis-48">
              <span className="field-label">New grouping</span>
              <input
                required
                value={groupingName}
                onChange={(e) => setGroupingName(e.target.value)}
                className="input"
              />
            </label>
            <button type="submit" className="btn">
              Create grouping
            </button>
          </form>
        ) : null}
      </section>
    </main>
  );
}
