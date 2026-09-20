"use client";

import { useEffect, useState } from "react";

import { getMyGrades, type UserGradeReport } from "@/lib/api";
import { fmt, pct } from "@/components/grades/format";

export function MyGradesView({ token, courseId }: { token: string; courseId: string }) {
  const [report, setReport] = useState<UserGradeReport | null>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    getMyGrades(token, courseId)
      .then(setReport)
      .catch(() => setError("Could not load your grades"));
  }, [token, courseId]);

  if (error) {
    return (
      <p role="alert" className="notice">
        {error}
      </p>
    );
  }
  if (!report) return <p className="text-muted">Loading…</p>;

  return (
    <section aria-labelledby="my-grades" className="flex flex-col gap-6">
      <h2 id="my-grades" className="border-b-2 border-ink pb-2 text-2xl">
        Your grades
      </h2>

      {report.items.length === 0 ? (
        <p className="text-muted">Nothing has been graded in this course yet.</p>
      ) : (
        <div className="overflow-x-auto">
          <table className="w-full min-w-[32rem] text-left text-sm">
            <thead>
              <tr className="border-b-2 border-ink">
                <th className="py-2 pr-4 font-medium">Item</th>
                <th className="py-2 pr-4 font-medium">Grade</th>
                <th className="py-2 pr-4 font-medium">Range</th>
                <th className="py-2 pr-4 font-medium">Percentage</th>
                <th className="py-2 pr-4 font-medium">Letter</th>
                <th className="py-2 font-medium">Feedback</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-rule">
              {report.items.map((row) => (
                <tr key={row.item.id}>
                  <td className="py-3 pr-4">{row.item.name}</td>
                  <td className="py-3 pr-4">
                    {row.hidden ? <span className="meta">Hidden</span> : fmt(row.grade)}
                    {row.passed === false ? <span className="meta ml-2">Below pass</span> : null}
                  </td>
                  <td className="py-3 pr-4 text-muted">
                    {fmt(row.item.min_grade)}–{fmt(row.item.max_grade)}
                  </td>
                  <td className="py-3 pr-4">{row.hidden ? "–" : pct(row.percentage)}</td>
                  <td className="py-3 pr-4">{row.hidden ? "–" : row.letter || "–"}</td>
                  <td className="py-3 text-muted">{row.feedback}</td>
                </tr>
              ))}
            </tbody>
            <tfoot>
              <tr className="border-t-2 border-ink font-medium">
                <td className="py-3 pr-4">Course total</td>
                <td className="py-3 pr-4">{fmt(report.course_total.grade)}</td>
                <td className="py-3 pr-4 text-muted">0–{fmt(report.course_total.max)}</td>
                <td className="py-3 pr-4">{pct(report.course_total.percentage)}</td>
                <td className="py-3 pr-4">{report.course_total.letter || "–"}</td>
                <td />
              </tr>
            </tfoot>
          </table>
        </div>
      )}
    </section>
  );
}
