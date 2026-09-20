"use client";

import Link from "next/link";
import { useParams, useRouter } from "next/navigation";
import { useCallback, useEffect, useState, type FormEvent } from "react";

import {
  ApiError,
  createSection,
  getCourse,
  getCourseContent,
  type Course,
  type SectionContent,
} from "@/lib/api";
import { useAuth } from "@/lib/auth-context";
import { useCourseCapabilities } from "@/lib/use-capabilities";
import { AddBookChapterForm } from "@/components/course/AddBookChapterForm";
import { AddFolderFileForm } from "@/components/course/AddFolderFileForm";
import { AddModuleForm } from "@/components/course/AddModuleForm";
import { CourseSettings } from "@/components/course/CourseSettings";
import { CourseTabs } from "@/components/course/CourseTabs";
import { ModuleControls } from "@/components/course/ModuleControls";
import { ModuleView } from "@/components/course/ModuleView";
import { ParticipantsPanel } from "@/components/course/ParticipantsPanel";
import { SectionHeader } from "@/components/course/SectionHeader";

export default function CourseDetailPage() {
  const { id } = useParams<{ id: string }>();
  const { user, token, loading } = useAuth();
  const router = useRouter();
  const [course, setCourse] = useState<Course | null>(null);
  const [sections, setSections] = useState<SectionContent[] | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [newSection, setNewSection] = useState("");
  const [sectionError, setSectionError] = useState<string | null>(null);
  const { can, ready } = useCourseCapabilities(token, id);

  const canEditCourse = can("course:update");
  const canManageModules = can("mod:manage");

  useEffect(() => {
    if (!loading && !user) router.push("/login");
  }, [loading, user, router]);

  const refresh = useCallback(() => {
    if (!token) return;
    getCourseContent(token, id)
      .then(setSections)
      .catch((err) =>
        setError(
          err instanceof ApiError && err.status === 403
            ? "You are not enrolled in this course. Ask your teacher or an admin to add you."
            : "Could not load course content",
        ),
      );
  }, [token, id]);

  useEffect(() => {
    if (!token) return;
    getCourse(token, id)
      .then(setCourse)
      .catch(() => setCourse(null));
    refresh();
  }, [token, id, refresh]);

  async function handleAddSection(e: FormEvent) {
    e.preventDefault();
    if (!token) return;
    setSectionError(null);
    try {
      await createSection(token, id, newSection.trim());
      setNewSection("");
      refresh();
    } catch (err) {
      setSectionError(err instanceof ApiError ? err.message : "Could not add the section");
    }
  }

  if (loading || !user) return null;

  if (error) {
    return (
      <main className="frame flex-1 py-12">
        <p role="alert" className="notice">
          {error}
        </p>
      </main>
    );
  }

  return (
    <main className="frame flex flex-1 flex-col gap-10 py-12">
      <header className="flex flex-col gap-4">
        <Link href="/courses" className="link self-start text-sm">
          ← My courses
        </Link>
        <div className="flex flex-wrap items-end justify-between gap-x-8 gap-y-4">
          <div className="min-w-0 max-w-2xl">
            <h1 className="text-display">{course?.title ?? "Loading…"}</h1>
            {course ? (
              <p className="meta mt-2">
                {course.short_name}
                {!course.visible ? " · Hidden from students" : ""}
              </p>
            ) : null}
            {course?.description ? (
              <p className="prose-measure mt-3 text-lg text-muted">{course.description}</p>
            ) : null}
          </div>
        </div>
        <CourseTabs courseId={id} />
        {canEditCourse && course && token ? (
          <CourseSettings key={course.id + course.title} token={token} course={course} onSaved={setCourse} />
        ) : null}
      </header>

      {sections === null ? (
        <p className="text-muted">Loading…</p>
      ) : sections.length === 0 ? (
        <div className="border-t-2 border-ink py-10">
          <p className="font-display text-xl">This course has no sections yet.</p>
        </div>
      ) : (
        <div className="flex flex-col gap-12">
          {sections.map((section) => (
            <section key={section.id} className="flex flex-col gap-4">
              {token ? (
                <SectionHeader
                  token={token}
                  section={section}
                  count={sections.length}
                  canManage={canEditCourse}
                  onChanged={refresh}
                />
              ) : null}

              {section.modules.length === 0 ? (
                <p className="text-muted">No content in this section yet.</p>
              ) : (
                <div className="flex flex-col divide-y divide-rule">
                  {section.modules.map((module, index) => (
                    <ModuleView
                      key={module.id}
                      module={module}
                      extra={
                        canManageModules && token && module.module_type === "folder" ? (
                          <AddFolderFileForm token={token} moduleId={module.id} onAdded={refresh} />
                        ) : canManageModules && token && module.module_type === "book" ? (
                          <AddBookChapterForm token={token} moduleId={module.id} onAdded={refresh} />
                        ) : undefined
                      }
                      manage={
                        canManageModules && token ? (
                          <ModuleControls
                            token={token}
                            module={module}
                            sections={sections}
                            index={index}
                            count={section.modules.length}
                            onChanged={refresh}
                          />
                        ) : undefined
                      }
                    />
                  ))}
                </div>
              )}

              {canManageModules && token ? (
                <AddModuleForm token={token} sectionId={section.id} onAdded={refresh} />
              ) : null}
            </section>
          ))}
        </div>
      )}

      {canEditCourse ? (
        <form onSubmit={handleAddSection} className="flex flex-wrap items-end gap-3 border-t border-rule pt-6">
          <label className="field min-w-0 flex-1 basis-56">
            <span className="field-label">Add a section</span>
            <input
              value={newSection}
              onChange={(e) => setNewSection(e.target.value)}
              placeholder={course?.format === "weeks" ? "Week 4" : "Topic 4"}
              className="input"
            />
          </label>
          <button type="submit" className="btn">
            Add section
          </button>
          {sectionError ? (
            <p role="alert" className="notice basis-full">
              {sectionError}
            </p>
          ) : null}
        </form>
      ) : null}

      {ready && can("course:viewparticipants") && token && course ? (
        <ParticipantsPanel
          token={token}
          courseId={id}
          ownerId={course.owner_id}
          canEnrol={can("enrol:manage")}
          canAssignRoles={can("role:assign")}
        />
      ) : null}
    </main>
  );
}
