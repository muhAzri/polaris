"use client";

import { useCallback, useEffect, useMemo, useState, type FormEvent } from "react";

import {
  ApiError,
  createCourseEvent,
  createMyEvent,
  deleteEvent,
  listCourseEvents,
  listMyEvents,
  type CalendarEvent,
  type RepeatRule,
} from "@/lib/api";

const WEEKDAYS = ["Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun"];

const sameDay = (a: Date, b: Date) =>
  a.getFullYear() === b.getFullYear() && a.getMonth() === b.getMonth() && a.getDate() === b.getDate();

const KIND_LABEL: Record<CalendarEvent["event_type"], string> = {
  course: "Course",
  user: "Personal",
  site: "Site",
};

/**
 * Month grid plus an event list. With a courseId it shows that course's
 * events; without one it shows the signed-in user's whole calendar.
 * `canCreate` gates the new-event form for course calendars (a personal
 * calendar is always editable by its owner).
 */
export function CalendarView({
  token,
  courseId,
  canCreate,
}: {
  token: string;
  courseId?: string;
  canCreate: boolean;
}) {
  const [month, setMonth] = useState(() => {
    const now = new Date();
    return new Date(now.getFullYear(), now.getMonth(), 1);
  });
  const [events, setEvents] = useState<CalendarEvent[] | null>(null);
  const [error, setError] = useState<string | null>(null);

  const [name, setName] = useState("");
  const [start, setStart] = useState("");
  const [end, setEnd] = useState("");
  const [repeat, setRepeat] = useState<RepeatRule>("none");
  const [repeatUntil, setRepeatUntil] = useState("");

  const range = useMemo(() => {
    const from = new Date(month.getFullYear(), month.getMonth(), 1);
    const to = new Date(month.getFullYear(), month.getMonth() + 1, 0, 23, 59, 59);
    return { from, to };
  }, [month]);

  const load = useCallback(() => {
    const fetcher = courseId
      ? listCourseEvents(token, courseId, range.from, range.to)
      : listMyEvents(token, range.from, range.to);
    fetcher.then(setEvents).catch(() => setError("Could not load events"));
  }, [token, courseId, range]);

  useEffect(load, [load]);

  const cells = useMemo(() => {
    const first = range.from;
    const offset = (first.getDay() + 6) % 7; // Monday first
    const days = new Date(first.getFullYear(), first.getMonth() + 1, 0).getDate();
    const out: (Date | null)[] = Array.from({ length: offset }, () => null);
    for (let d = 1; d <= days; d++) out.push(new Date(first.getFullYear(), first.getMonth(), d));
    while (out.length % 7 !== 0) out.push(null);
    return out;
  }, [range]);

  const eventsOn = (day: Date) =>
    (events ?? []).filter((e) => sameDay(new Date(e.start_at), day));

  const canDelete = (e: CalendarEvent) =>
    !e.course_module_id && (e.event_type === "user" || (e.event_type === "course" && canCreate));

  async function handleCreate(e: FormEvent) {
    e.preventDefault();
    setError(null);
    const body = {
      name: name.trim(),
      start_at: new Date(start).toISOString(),
      end_at: end ? new Date(end).toISOString() : undefined,
      repeat_rule: repeat,
      // A date picker means "through the end of that day".
      repeat_until:
        repeat !== "none" && repeatUntil ? new Date(`${repeatUntil}T23:59:59`).toISOString() : undefined,
    };
    try {
      if (courseId) await createCourseEvent(token, courseId, body);
      else await createMyEvent(token, body);
      setName("");
      setStart("");
      setEnd("");
      setRepeat("none");
      setRepeatUntil("");
      load();
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Could not create the event");
    }
  }

  async function handleDelete(event: CalendarEvent) {
    const message = event.repeat_rule !== "none" ? "Delete this event and all its repeats?" : "Delete this event?";
    if (!window.confirm(message)) return;
    try {
      await deleteEvent(token, event.id);
      load();
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Could not delete the event");
    }
  }

  const today = new Date();
  const monthLabel = month.toLocaleDateString(undefined, { month: "long", year: "numeric" });

  return (
    <section aria-labelledby="calendar" className="flex flex-col gap-8">
      <div className="flex flex-wrap items-end justify-between gap-4 border-b-2 border-ink pb-2">
        <h2 id="calendar" className="text-2xl">
          {monthLabel}
        </h2>
        <div className="flex gap-2">
          <button
            type="button"
            className="btn btn-quiet btn-sm"
            onClick={() => setMonth(new Date(month.getFullYear(), month.getMonth() - 1, 1))}
          >
            ← Previous
          </button>
          <button
            type="button"
            className="btn btn-quiet btn-sm"
            onClick={() => setMonth(new Date(today.getFullYear(), today.getMonth(), 1))}
          >
            Today
          </button>
          <button
            type="button"
            className="btn btn-quiet btn-sm"
            onClick={() => setMonth(new Date(month.getFullYear(), month.getMonth() + 1, 1))}
          >
            Next →
          </button>
        </div>
      </div>

      {error ? (
        <p role="alert" className="notice">
          {error}
        </p>
      ) : null}

      <div className="overflow-x-auto">
        <div role="grid" aria-label={monthLabel} className="grid min-w-[38rem] grid-cols-7 border-l border-t border-rule">
          {WEEKDAYS.map((d) => (
            <div key={d} role="columnheader" className="meta border-b border-r border-rule bg-paper-2 px-2 py-1">
              {d}
            </div>
          ))}
          {cells.map((day, i) => (
            <div
              key={i}
              role="gridcell"
              className={`min-h-24 border-b border-r border-rule p-2 ${day ? "" : "bg-paper-2"}`}
            >
              {day ? (
                <>
                  <span
                    className={`text-sm ${sameDay(day, today) ? "font-semibold text-accent" : "text-muted"}`}
                    aria-label={sameDay(day, today) ? `${day.getDate()}, today` : undefined}
                  >
                    {day.getDate()}
                  </span>
                  <ul className="mt-1 flex flex-col gap-1">
                    {eventsOn(day).map((e, n) => (
                      <li
                        key={`${e.id}-${n}`}
                        title={`${KIND_LABEL[e.event_type]}: ${e.name}`}
                        className={`truncate border-l-2 pl-1 text-xs ${
                          e.event_type === "course"
                            ? "border-accent"
                            : e.event_type === "site"
                              ? "border-ink"
                              : "border-rule-strong"
                        }`}
                      >
                        {e.name}
                      </li>
                    ))}
                  </ul>
                </>
              ) : null}
            </div>
          ))}
        </div>
      </div>

      <div className="split">
        <div className="flex flex-col gap-3">
          <h3 className="font-display text-lg font-medium">This month</h3>
          {events === null ? (
            <p className="text-muted">Loading…</p>
          ) : events.length === 0 ? (
            <p className="text-muted">Nothing scheduled.</p>
          ) : (
            <ul className="ledger">
              {events.map((e, n) => (
                <li key={`${e.id}-${n}`} className="flex flex-wrap items-center justify-between gap-3 py-3">
                  <span className="flex min-w-0 flex-col">
                    <span>{e.name}</span>
                    <span className="meta normal-case">
                      {KIND_LABEL[e.event_type]} · {new Date(e.start_at).toLocaleString()}
                      {e.repeat_rule !== "none" ? ` · repeats ${e.repeat_rule}` : ""}
                    </span>
                  </span>
                  {canDelete(e) ? (
                    <button type="button" className="btn btn-quiet btn-sm" onClick={() => handleDelete(e)}>
                      Delete
                    </button>
                  ) : null}
                </li>
              ))}
            </ul>
          )}
        </div>

        {courseId && !canCreate ? null : (
          <form onSubmit={handleCreate} className="flex flex-col gap-4">
            <h3 className="font-display text-lg font-medium">
              {courseId ? "New course event" : "New personal event"}
            </h3>
            <label className="field">
              <span className="field-label">Name</span>
              <input required value={name} onChange={(e) => setName(e.target.value)} className="input" />
            </label>
            <label className="field">
              <span className="field-label">Starts</span>
              <input
                required
                type="datetime-local"
                value={start}
                onChange={(e) => setStart(e.target.value)}
                className="input"
              />
            </label>
            <label className="field">
              <span className="field-label">Ends (optional)</span>
              <input type="datetime-local" value={end} onChange={(e) => setEnd(e.target.value)} className="input" />
            </label>
            <label className="field">
              <span className="field-label">Repeat</span>
              <select value={repeat} onChange={(e) => setRepeat(e.target.value as RepeatRule)} className="input">
                <option value="none">Does not repeat</option>
                <option value="daily">Daily</option>
                <option value="weekly">Weekly</option>
                <option value="monthly">Monthly</option>
              </select>
            </label>
            {repeat !== "none" ? (
              <label className="field">
                <span className="field-label">Repeat until</span>
                <input
                  type="date"
                  value={repeatUntil}
                  onChange={(e) => setRepeatUntil(e.target.value)}
                  className="input"
                />
              </label>
            ) : null}
            <button type="submit" className="btn btn-primary self-start">
              Add event
            </button>
          </form>
        )}
      </div>
    </section>
  );
}
