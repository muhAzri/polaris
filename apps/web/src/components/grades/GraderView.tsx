"use client";

import { useCallback, useEffect, useMemo, useState, type FormEvent } from "react";

import {
  ApiError,
  createGradeCategory,
  createGradeItem,
  deleteGradeCategory,
  deleteGradeItem,
  getGraderReport,
  setGrade,
  updateGradeCategory,
  updateGradeItem,
  type AggregationMethod,
  type GradeCategory,
  type GraderReport,
} from "@/lib/api";
import { AGGREGATIONS, fmt, pct } from "@/components/grades/format";

function flatten(categories: GradeCategory[]) {
  const children = new Map<string | null, GradeCategory[]>();
  for (const c of categories) {
    const key = c.is_root ? null : c.parent_id;
    children.set(key, [...(children.get(key) ?? []), c]);
  }
  const out: { category: GradeCategory; depth: number }[] = [];
  const walk = (parent: string | null, depth: number) => {
    for (const c of children.get(parent) ?? []) {
      out.push({ category: c, depth });
      walk(c.id, depth + 1);
    }
  };
  walk(null, 0);
  return out;
}

export function GraderView({
  token,
  courseId,
  canManage,
  canEdit,
}: {
  token: string;
  courseId: string;
  canManage: boolean;
  canEdit: boolean;
}) {
  const [report, setReport] = useState<GraderReport | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [catName, setCatName] = useState("");
  const [catMethod, setCatMethod] = useState<AggregationMethod>("mean");
  const [catParent, setCatParent] = useState("");
  const [itemName, setItemName] = useState("");
  const [itemMax, setItemMax] = useState("100");
  const [itemCategory, setItemCategory] = useState("");

  const load = useCallback(() => {
    getGraderReport(token, courseId)
      .then(setReport)
      .catch(() => setError("Could not load the grader report"));
  }, [token, courseId]);

  useEffect(load, [load]);

  const tree = useMemo(() => flatten(report?.categories ?? []), [report]);

  async function run(action: () => Promise<unknown>, failure: string) {
    setError(null);
    try {
      await action();
      load();
    } catch (err) {
      setError(err instanceof ApiError ? err.message : failure);
    }
  }

  async function handleGrade(itemId: string, userId: string, raw: string, current: number | null) {
    const next = raw.trim() === "" ? null : Number(raw);
    if (next === current || (next !== null && Number.isNaN(next))) return;
    await run(() => setGrade(token, itemId, userId, { grade: next }), "Could not save the grade");
  }

  function handleAddCategory(e: FormEvent) {
    e.preventDefault();
    return run(async () => {
      await createGradeCategory(token, courseId, {
        name: catName.trim(),
        aggregation: catMethod,
        parent_id: catParent || undefined,
      });
      setCatName("");
    }, "Could not add the category");
  }

  function handleAddItem(e: FormEvent) {
    e.preventDefault();
    return run(async () => {
      await createGradeItem(token, courseId, {
        name: itemName.trim(),
        max_grade: Number(itemMax) || 100,
        category_id: itemCategory || undefined,
      });
      setItemName("");
    }, "Could not add the item");
  }

  if (!report) {
    return error ? (
      <p role="alert" className="notice">
        {error}
      </p>
    ) : (
      <p className="text-muted">Loading…</p>
    );
  }

  const categoryName = (id: string) => report.categories.find((c) => c.id === id)?.name ?? "";

  return (
    <div className="flex flex-col gap-12">
      {error ? (
        <p role="alert" className="notice">
          {error}
        </p>
      ) : null}

      <section aria-labelledby="grader" className="flex flex-col gap-4">
        <h2 id="grader" className="border-b-2 border-ink pb-2 text-2xl">
          Grader report
        </h2>
        {report.students.length === 0 ? (
          <p className="text-muted">No students are enrolled yet.</p>
        ) : report.items.length === 0 ? (
          <p className="text-muted">There are no grade items yet. Add one below.</p>
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-left text-sm">
              <thead>
                <tr className="border-b-2 border-ink align-bottom">
                  <th className="py-2 pr-4 font-medium">Student</th>
                  {report.items.map((item) => (
                    <th key={item.id} className="min-w-28 py-2 pr-4 font-medium">
                      <span className="flex flex-col gap-1">
                        <span>
                          {item.name}
                          {item.hidden ? <span className="meta ml-2">Hidden</span> : null}
                        </span>
                        <span className="meta normal-case">
                          {categoryName(item.category_id)} · /{fmt(item.max_grade)}
                        </span>
                        {canManage ? (
                          <span className="flex gap-2">
                            <button
                              type="button"
                              className="link text-xs"
                              onClick={() =>
                                run(
                                  () => updateGradeItem(token, item.id, { hidden: !item.hidden }),
                                  "Could not update the item",
                                )
                              }
                            >
                              {item.hidden ? "Show" : "Hide"}
                            </button>
                            {!item.course_module_id ? (
                              <button
                                type="button"
                                className="link text-xs"
                                onClick={() => {
                                  if (window.confirm(`Delete "${item.name}" and its grades?`)) {
                                    run(() => deleteGradeItem(token, item.id), "Could not delete the item");
                                  }
                                }}
                              >
                                Delete
                              </button>
                            ) : null}
                          </span>
                        ) : null}
                      </span>
                    </th>
                  ))}
                  <th className="py-2 font-medium">Course total</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-rule">
                {report.students.map((student) => (
                  <tr key={student.user_id}>
                    <td className="py-2 pr-4">{student.name}</td>
                    {report.items.map((item) => {
                      const cell = student.items.find((r) => r.item.id === item.id);
                      return (
                        <td key={item.id} className="py-2 pr-4">
                          {canEdit ? (
                            <input
                              key={`${student.user_id}-${item.id}-${cell?.grade ?? ""}`}
                              type="number"
                              step="any"
                              aria-label={`${student.name}, ${item.name}`}
                              defaultValue={cell?.grade ?? ""}
                              disabled={cell?.locked}
                              onBlur={(e) =>
                                handleGrade(item.id, student.user_id, e.target.value, cell?.grade ?? null)
                              }
                              className="input w-24 py-1"
                            />
                          ) : (
                            fmt(cell?.grade)
                          )}
                          {cell?.locked ? <span className="meta ml-1">Locked</span> : null}
                          {cell?.excluded ? <span className="meta ml-1">Excluded</span> : null}
                        </td>
                      );
                    })}
                    <td className="py-2 font-medium">
                      {fmt(student.course_total.grade)}{" "}
                      <span className="meta">
                        {pct(student.course_total.percentage)} {student.course_total.letter}
                      </span>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </section>

      {canManage ? (
        <section aria-labelledby="structure" className="flex flex-col gap-6">
          <h2 id="structure" className="border-b-2 border-ink pb-2 text-2xl">
            Gradebook setup
          </h2>

          <ul className="ledger">
            {tree.map(({ category, depth }) => (
              <li
                key={category.id}
                style={{ paddingLeft: `${depth * 1.5}rem` }}
                className="flex flex-wrap items-center justify-between gap-3 py-3"
              >
                <span className="flex flex-col">
                  <span className="ledger-title">
                    {category.name}
                    {category.hidden ? <span className="meta ml-3">Hidden</span> : null}
                  </span>
                  <span className="meta normal-case">
                    {category.is_root ? "Course total" : "Category"} · max {fmt(category.max_grade)}
                    {category.drop_lowest > 0 ? ` · drops lowest ${category.drop_lowest}` : ""}
                  </span>
                </span>
                <span className="flex flex-wrap items-center gap-2">
                  <select
                    aria-label={`Aggregation for ${category.name}`}
                    value={category.aggregation}
                    onChange={(e) =>
                      run(
                        () =>
                          updateGradeCategory(token, category.id, {
                            aggregation: e.target.value as AggregationMethod,
                          }),
                        "Could not change the aggregation",
                      )
                    }
                    className="input w-auto py-1 text-sm"
                  >
                    {AGGREGATIONS.map((a) => (
                      <option key={a.value} value={a.value}>
                        {a.label}
                      </option>
                    ))}
                  </select>
                  {!category.is_root ? (
                    <button
                      type="button"
                      className="btn btn-quiet btn-sm"
                      onClick={() => {
                        if (window.confirm(`Delete "${category.name}"? Its items move up one level.`)) {
                          run(() => deleteGradeCategory(token, category.id), "Could not delete the category");
                        }
                      }}
                    >
                      Delete
                    </button>
                  ) : null}
                </span>
              </li>
            ))}
          </ul>

          <div className="split">
            <form onSubmit={handleAddCategory} className="flex flex-col gap-4">
              <h3 className="font-display text-lg font-medium">Add a category</h3>
              <label className="field">
                <span className="field-label">Name</span>
                <input required value={catName} onChange={(e) => setCatName(e.target.value)} className="input" />
              </label>
              <label className="field">
                <span className="field-label">Aggregation</span>
                <select
                  value={catMethod}
                  onChange={(e) => setCatMethod(e.target.value as AggregationMethod)}
                  className="input"
                >
                  {AGGREGATIONS.map((a) => (
                    <option key={a.value} value={a.value}>
                      {a.label}
                    </option>
                  ))}
                </select>
              </label>
              <label className="field">
                <span className="field-label">Parent</span>
                <select value={catParent} onChange={(e) => setCatParent(e.target.value)} className="input">
                  <option value="">Course total</option>
                  {tree
                    .filter((t) => !t.category.is_root)
                    .map(({ category }) => (
                      <option key={category.id} value={category.id}>
                        {category.name}
                      </option>
                    ))}
                </select>
              </label>
              <button type="submit" className="btn self-start">
                Add category
              </button>
            </form>

            <form onSubmit={handleAddItem} className="flex flex-col gap-4">
              <h3 className="font-display text-lg font-medium">Add a grade item</h3>
              <label className="field">
                <span className="field-label">Name</span>
                <input required value={itemName} onChange={(e) => setItemName(e.target.value)} className="input" />
              </label>
              <label className="field">
                <span className="field-label">Maximum grade</span>
                <input
                  type="number"
                  min={1}
                  step="any"
                  value={itemMax}
                  onChange={(e) => setItemMax(e.target.value)}
                  className="input"
                />
              </label>
              <label className="field">
                <span className="field-label">Category</span>
                <select value={itemCategory} onChange={(e) => setItemCategory(e.target.value)} className="input">
                  <option value="">Course total</option>
                  {tree
                    .filter((t) => !t.category.is_root)
                    .map(({ category }) => (
                      <option key={category.id} value={category.id}>
                        {category.name}
                      </option>
                    ))}
                </select>
              </label>
              <button type="submit" className="btn self-start">
                Add item
              </button>
            </form>
          </div>
        </section>
      ) : null}
    </div>
  );
}
