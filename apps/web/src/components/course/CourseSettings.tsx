"use client";

import { useEffect, useState, type FormEvent } from "react";

import {
  ApiError,
  listCategories,
  updateCourse,
  type Category,
  type Course,
  type CourseFormat,
} from "@/lib/api";

const toDateInput = (iso: string | null) => (iso ? iso.slice(0, 10) : "");

export function CourseSettings({
  token,
  course,
  onSaved,
}: {
  token: string;
  course: Course;
  onSaved: (course: Course) => void;
}) {
  const [open, setOpen] = useState(false);
  const [title, setTitle] = useState(course.title);
  const [shortName, setShortName] = useState(course.short_name);
  const [idNumber, setIdNumber] = useState(course.id_number);
  const [description, setDescription] = useState(course.description);
  const [format, setFormat] = useState<CourseFormat>(course.format);
  const [visible, setVisible] = useState(course.visible);
  const [startDate, setStartDate] = useState(toDateInput(course.start_date));
  const [endDate, setEndDate] = useState(toDateInput(course.end_date));
  const [categoryId, setCategoryId] = useState(course.category_id ?? "");
  const [categories, setCategories] = useState<Category[]>([]);
  const [error, setError] = useState<string | null>(null);
  const [saved, setSaved] = useState(false);

  useEffect(() => {
    if (!open) return;
    listCategories(token)
      .then(setCategories)
      .catch(() => setCategories([]));
  }, [open, token]);

  async function handleSave(e: FormEvent) {
    e.preventDefault();
    setError(null);
    setSaved(false);
    try {
      const updated = await updateCourse(token, course.id, {
        title,
        short_name: shortName,
        id_number: idNumber,
        description,
        format,
        visible,
        category_id: categoryId || undefined,
        start_date: startDate ? new Date(startDate).toISOString() : null,
        end_date: endDate ? new Date(`${endDate}T23:59:59`).toISOString() : null,
      });
      onSaved(updated);
      setSaved(true);
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Could not save course settings");
    }
  }

  if (!open) {
    return (
      <button type="button" onClick={() => setOpen(true)} className="btn btn-quiet btn-sm self-start">
        Course settings
      </button>
    );
  }

  return (
    <form onSubmit={handleSave} className="flex flex-col gap-4 border-t-2 border-ink pt-5">
      <h2 className="text-2xl">Course settings</h2>
      <div className="grid gap-4 sm:grid-cols-2">
        <label className="field">
          <span className="field-label">Title</span>
          <input required value={title} onChange={(e) => setTitle(e.target.value)} className="input" />
        </label>
        <label className="field">
          <span className="field-label">Short name</span>
          <input
            required
            value={shortName}
            onChange={(e) => setShortName(e.target.value)}
            className="input"
          />
        </label>
        <label className="field">
          <span className="field-label">ID number</span>
          <input value={idNumber} onChange={(e) => setIdNumber(e.target.value)} className="input" />
        </label>
        <label className="field">
          <span className="field-label">Category</span>
          <select value={categoryId} onChange={(e) => setCategoryId(e.target.value)} className="input">
            <option value="">Keep current</option>
            {categories.map((c) => (
              <option key={c.id} value={c.id}>
                {c.name}
              </option>
            ))}
          </select>
        </label>
        <label className="field">
          <span className="field-label">Start date</span>
          <input
            type="date"
            value={startDate}
            onChange={(e) => setStartDate(e.target.value)}
            className="input"
          />
        </label>
        <label className="field">
          <span className="field-label">End date</span>
          <input
            type="date"
            value={endDate}
            onChange={(e) => setEndDate(e.target.value)}
            className="input"
          />
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
        <label className="flex items-center gap-3 self-end pb-2">
          <input
            type="checkbox"
            checked={visible}
            onChange={(e) => setVisible(e.target.checked)}
            className="size-4 accent-accent"
          />
          <span className="text-sm font-medium">Visible to students</span>
        </label>
      </div>
      <label className="field">
        <span className="field-label">Description</span>
        <textarea
          value={description}
          onChange={(e) => setDescription(e.target.value)}
          rows={3}
          className="input"
        />
      </label>
      {error ? (
        <p role="alert" className="notice">
          {error}
        </p>
      ) : null}
      {saved ? <p className="notice notice-success">Saved.</p> : null}
      <div className="flex gap-2">
        <button type="submit" className="btn btn-primary">
          Save settings
        </button>
        <button type="button" onClick={() => setOpen(false)} className="btn btn-quiet">
          Close
        </button>
      </div>
    </form>
  );
}
