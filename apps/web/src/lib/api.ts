const API_URL = process.env.NEXT_PUBLIC_API_URL ?? "http://localhost:8080";

export class ApiError extends Error {
  status: number;
  constructor(status: number, message: string) {
    super(message);
    this.status = status;
  }
}

type RequestOptions = Omit<RequestInit, "body"> & {
  token?: string;
  body?: BodyInit | Record<string, unknown>;
};

async function request<T>(path: string, options: RequestOptions = {}): Promise<T> {
  const { token, headers, body, ...rest } = options;
  const isFormData = body instanceof FormData;
  const res = await fetch(`${API_URL}${path}`, {
    ...rest,
    body: isFormData || typeof body === "string" || body === undefined ? body : JSON.stringify(body),
    headers: {
      ...(isFormData ? {} : { "Content-Type": "application/json" }),
      ...(token ? { Authorization: `Bearer ${token}` } : {}),
      ...headers,
    },
  });

  if (!res.ok) {
    const parsed = await res.json().catch(() => null);
    throw new ApiError(res.status, parsed?.error ?? `Request failed with status ${res.status}`);
  }
  if (res.status === 204) return undefined as T;
  return res.json();
}

export type Role = "student" | "teacher" | "admin";

export type User = {
  id: string;
  name: string;
  email: string;
  role: Role;
  created_at: string;
};

export type AuthResponse = { user: User; token: string };

export function register(name: string, email: string, password: string) {
  return request<AuthResponse>("/api/v1/auth/register", {
    method: "POST",
    body: { name, email, password },
  });
}

export function login(email: string, password: string) {
  return request<AuthResponse>("/api/v1/auth/login", {
    method: "POST",
    body: { email, password },
  });
}

export function me(token: string) {
  return request<User>("/api/v1/auth/me", { token });
}

export type CourseFormat = "topics" | "weeks";

export type Course = {
  id: string;
  owner_id: string;
  category_id: string | null;
  title: string;
  short_name: string;
  id_number: string;
  description: string;
  format: CourseFormat;
  start_date: string | null;
  end_date: string | null;
  visible: boolean;
  self_enrol: boolean;
  requires_key: boolean;
  created_at: string;
};

export type CourseInput = {
  title: string;
  short_name?: string;
  id_number?: string;
  description?: string;
  category_id?: string;
  format?: CourseFormat;
  start_date?: string | null;
  end_date?: string | null;
  visible?: boolean;
};

export function listCourses(token: string) {
  return request<Course[]>("/api/v1/courses", { token });
}

export function listAvailableCourses(token: string) {
  return request<Course[]>("/api/v1/courses/available", { token });
}

export function createCourse(token: string, input: CourseInput) {
  return request<Course>("/api/v1/courses", { method: "POST", token, body: input });
}

export function getCourse(token: string, courseId: string) {
  return request<Course>(`/api/v1/courses/${courseId}`, { token });
}

export function updateCourse(token: string, courseId: string, input: CourseInput) {
  return request<Course>(`/api/v1/courses/${courseId}`, { method: "PUT", token, body: input });
}

/** Capabilities the signed-in user holds in a course, e.g. "grade:manage". */
export function getMyCapabilities(token: string, courseId: string) {
  return request<string[]>(`/api/v1/courses/${courseId}/my-capabilities`, { token });
}

export type Category = {
  id: string;
  parent_id: string | null;
  name: string;
  description: string;
  visible: boolean;
  is_default: boolean;
  course_count: number;
};

export function listCategories(token: string) {
  return request<Category[]>("/api/v1/categories", { token });
}

export type Section = {
  id: string;
  course_id: string;
  title: string;
  summary: string;
  position: number;
  visible: boolean;
  created_at: string;
};

export function createSection(token: string, courseId: string, title: string, summary = "") {
  return request<Section>(`/api/v1/courses/${courseId}/sections`, {
    method: "POST",
    token,
    body: { title, summary },
  });
}

export function updateSection(
  token: string,
  sectionId: string,
  patch: { title?: string; summary?: string; visible?: boolean; position?: number },
) {
  return request<Section>(`/api/v1/sections/${sectionId}`, { method: "PATCH", token, body: patch });
}

export function deleteSection(token: string, sectionId: string) {
  return request<void>(`/api/v1/sections/${sectionId}`, { method: "DELETE", token });
}

export function enrollCourse(token: string, courseId: string, key = "") {
  return request<void>(`/api/v1/courses/${courseId}/enroll`, {
    method: "POST",
    token,
    body: { key },
  });
}

export type GroupMode = "none" | "separate" | "visible";

export type CourseModule = {
  id: string;
  section_id: string;
  module_type: string;
  instance_id: string;
  position: number;
  visible: boolean;
  intro: string;
  group_mode: GroupMode;
  grouping_id: string | null;
  available_from: string | null;
  available_until: string | null;
  created_at: string;
};

export type LabelData = { content: string };
export type PageData = { title: string; content: string };
export type UrlData = { title: string; url: string; description: string };
export type ResourceData = {
  title: string;
  file_name: string;
  file_size: number;
  content_type: string;
  download_url: string;
};
export type FolderFileData = {
  id: string;
  dir_path: string;
  file_name: string;
  file_size: number;
  content_type: string;
  download_url: string;
};
export type FolderData = { title: string; description: string; files: FolderFileData[] };
export type BookChapterData = {
  id: string;
  title: string;
  content: string;
  position: number;
  subchapter: boolean;
  hidden: boolean;
};
export type BookData = { title: string; intro: string; chapters: BookChapterData[] };

export type ModuleData = LabelData | PageData | UrlData | ResourceData | FolderData | BookData;

export type ModuleContent = {
  id: string;
  module_type: string;
  position: number;
  visible: boolean;
  intro: string;
  group_mode: GroupMode;
  grouping_id: string | null;
  available_from: string | null;
  available_until: string | null;
  /** True when the viewer cannot open it yet; `data` is then null. */
  restricted: boolean;
  data: ModuleData | null;
};

export type SectionContent = {
  id: string;
  title: string;
  summary: string;
  position: number;
  visible: boolean;
  modules: ModuleContent[];
};

export function getCourseContent(token: string, courseId: string) {
  return request<SectionContent[]>(`/api/v1/courses/${courseId}/content`, { token });
}

export type Participant = {
  user_id: string;
  name: string;
  email: string;
  roles: string[];
  method: string;
  status: "active" | "suspended";
  time_start: string | null;
  time_end: string | null;
  enrolled_at: string;
};

export function listParticipants(token: string, courseId: string) {
  return request<Participant[]>(`/api/v1/courses/${courseId}/participants`, { token });
}

export function enrolUser(token: string, courseId: string, email: string, role = "student") {
  return request<void>(`/api/v1/courses/${courseId}/enrolments`, {
    method: "POST",
    token,
    body: { email, role },
  });
}

export function updateEnrolment(
  token: string,
  courseId: string,
  userId: string,
  patch: { status?: "active" | "suspended"; time_start?: string; time_end?: string; clear_end?: boolean },
) {
  return request<void>(`/api/v1/courses/${courseId}/enrolments/${userId}`, {
    method: "PATCH",
    token,
    body: patch,
  });
}

export type RoleInfo = {
  id: string;
  name: string;
  display_name: string;
  description: string;
  is_system: boolean;
};

export function listRoles(token: string) {
  return request<RoleInfo[]>("/api/v1/roles", { token });
}

export function assignCourseRole(token: string, courseId: string, userId: string, role: string) {
  return request<void>(`/api/v1/courses/${courseId}/role-assignments`, {
    method: "POST",
    token,
    body: { user_id: userId, role },
  });
}

export function removeCourseRole(token: string, courseId: string, userId: string, role: string) {
  return request<void>(`/api/v1/courses/${courseId}/role-assignments/${userId}/${role}`, {
    method: "DELETE",
    token,
  });
}

export function unenrolUser(token: string, courseId: string, userId: string) {
  return request<void>(`/api/v1/courses/${courseId}/enrolments/${userId}`, {
    method: "DELETE",
    token,
  });
}

export type SelfEnrolment = {
  enabled: boolean;
  key: string;
  max_users: number | null;
  enrol_start: string | null;
  enrol_end: string | null;
  duration_days: number | null;
  role: string;
};

export function getSelfEnrolment(token: string, courseId: string) {
  return request<SelfEnrolment>(`/api/v1/courses/${courseId}/self-enrolment`, { token });
}

export function setSelfEnrolment(token: string, courseId: string, settings: SelfEnrolment) {
  return request<void>(`/api/v1/courses/${courseId}/self-enrolment`, {
    method: "PUT",
    token,
    body: settings,
  });
}

export function createLabel(token: string, sectionId: string, content: string) {
  return request<{ module: CourseModule }>(`/api/v1/sections/${sectionId}/modules/label`, {
    method: "POST",
    token,
    body: { content },
  });
}

export function createPage(token: string, sectionId: string, title: string, content: string) {
  return request<{ module: CourseModule }>(`/api/v1/sections/${sectionId}/modules/page`, {
    method: "POST",
    token,
    body: { title, content },
  });
}

export function createUrl(
  token: string,
  sectionId: string,
  title: string,
  url: string,
  description: string,
) {
  return request<{ module: CourseModule }>(`/api/v1/sections/${sectionId}/modules/url`, {
    method: "POST",
    token,
    body: { title, url, description },
  });
}

export function createResource(token: string, sectionId: string, title: string, file: File) {
  const form = new FormData();
  form.append("title", title);
  form.append("file", file);
  return request<{ module: CourseModule }>(`/api/v1/sections/${sectionId}/modules/resource`, {
    method: "POST",
    token,
    body: form,
  });
}

export function createFolder(
  token: string,
  sectionId: string,
  title: string,
  description: string,
) {
  return request<{ module: CourseModule }>(`/api/v1/sections/${sectionId}/modules/folder`, {
    method: "POST",
    token,
    body: { title, description },
  });
}

export function addFolderFile(token: string, moduleId: string, file: File, dirPath = "/") {
  const form = new FormData();
  form.append("file", file);
  form.append("path", dirPath);
  return request(`/api/v1/modules/${moduleId}/folder-files`, {
    method: "POST",
    token,
    body: form,
  });
}

export function createBook(token: string, sectionId: string, title: string, intro: string) {
  return request<{ module: CourseModule }>(`/api/v1/sections/${sectionId}/modules/book`, {
    method: "POST",
    token,
    body: { title, intro },
  });
}

export function addBookChapter(
  token: string,
  moduleId: string,
  title: string,
  content: string,
  subchapter = false,
) {
  return request(`/api/v1/modules/${moduleId}/book-chapters`, {
    method: "POST",
    token,
    body: { title, content, subchapter },
  });
}

export type ModulePatch = {
  visible?: boolean;
  intro?: string;
  group_mode?: GroupMode;
  grouping_id?: string;
  clear_grouping?: boolean;
  available_from?: string;
  available_until?: string;
  clear_availability?: boolean;
  section_id?: string;
  position?: number;
};

export function updateModule(token: string, moduleId: string, patch: ModulePatch) {
  return request<CourseModule>(`/api/v1/modules/${moduleId}`, { method: "PATCH", token, body: patch });
}

export function updateModuleContent(token: string, moduleId: string, fields: Record<string, string>) {
  return request<void>(`/api/v1/modules/${moduleId}/content`, { method: "PUT", token, body: fields });
}

export function deleteModule(token: string, moduleId: string) {
  return request<void>(`/api/v1/modules/${moduleId}`, { method: "DELETE", token });
}

export function deleteFolderFile(token: string, moduleId: string, fileId: string) {
  return request<void>(`/api/v1/modules/${moduleId}/folder-files/${fileId}`, {
    method: "DELETE",
    token,
  });
}

export function updateBookChapter(
  token: string,
  moduleId: string,
  chapterId: string,
  patch: { title?: string; content?: string; subchapter?: boolean; hidden?: boolean; position?: number },
) {
  return request(`/api/v1/modules/${moduleId}/book-chapters/${chapterId}`, {
    method: "PATCH",
    token,
    body: patch,
  });
}

export function deleteBookChapter(token: string, moduleId: string, chapterId: string) {
  return request<void>(`/api/v1/modules/${moduleId}/book-chapters/${chapterId}`, {
    method: "DELETE",
    token,
  });
}

// --- Gradebook ---

export type AggregationMethod = "mean" | "weighted" | "natural" | "min" | "max" | "median" | "mode";

export type GradeCategory = {
  id: string;
  course_id: string;
  parent_id: string | null;
  is_root: boolean;
  name: string;
  aggregation: AggregationMethod;
  max_grade: number;
  weight: number | null;
  drop_lowest: number;
  hidden: boolean;
};

export type GradeItem = {
  id: string;
  course_id: string;
  category_id: string;
  course_module_id: string | null;
  name: string;
  min_grade: number;
  max_grade: number;
  pass_grade: number | null;
  weight: number | null;
  hidden: boolean;
};

export type GradeTotal = {
  grade: number | null;
  max: number;
  percentage: number | null;
  letter: string;
};

export type ItemResult = {
  item: GradeItem;
  grade: number | null;
  feedback: string;
  percentage: number | null;
  letter: string;
  passed: boolean | null;
  hidden: boolean;
  locked: boolean;
  overridden: boolean;
  excluded: boolean;
};

export type UserGradeReport = {
  user_id: string;
  name: string;
  email: string;
  items: ItemResult[];
  categories: Record<string, GradeTotal>;
  course_total: GradeTotal;
};

export type GraderReport = {
  categories: GradeCategory[];
  items: GradeItem[];
  students: UserGradeReport[];
};

export function getMyGrades(token: string, courseId: string) {
  return request<UserGradeReport>(`/api/v1/courses/${courseId}/gradebook/mine`, { token });
}

export function getGraderReport(token: string, courseId: string) {
  return request<GraderReport>(`/api/v1/courses/${courseId}/gradebook/report`, { token });
}

export function createGradeCategory(
  token: string,
  courseId: string,
  body: { name: string; aggregation?: AggregationMethod; parent_id?: string; drop_lowest?: number },
) {
  return request<GradeCategory>(`/api/v1/courses/${courseId}/gradebook/categories`, {
    method: "POST",
    token,
    body,
  });
}

export function updateGradeCategory(
  token: string,
  categoryId: string,
  body: { name?: string; aggregation?: AggregationMethod; drop_lowest?: number; hidden?: boolean },
) {
  return request<GradeCategory>(`/api/v1/gradebook/categories/${categoryId}`, {
    method: "PATCH",
    token,
    body,
  });
}

export function deleteGradeCategory(token: string, categoryId: string) {
  return request<void>(`/api/v1/gradebook/categories/${categoryId}`, { method: "DELETE", token });
}

export function createGradeItem(
  token: string,
  courseId: string,
  body: { name: string; category_id?: string; max_grade?: number; pass_grade?: number; weight?: number },
) {
  return request<GradeItem>(`/api/v1/courses/${courseId}/gradebook/items`, {
    method: "POST",
    token,
    body,
  });
}

export function updateGradeItem(token: string, itemId: string, body: { hidden?: boolean; name?: string }) {
  return request<GradeItem>(`/api/v1/gradebook/items/${itemId}`, { method: "PATCH", token, body });
}

export function deleteGradeItem(token: string, itemId: string) {
  return request<void>(`/api/v1/gradebook/items/${itemId}`, { method: "DELETE", token });
}

/** `grade: null` clears the grade; omit it to change only the flags. */
export function setGrade(
  token: string,
  itemId: string,
  userId: string,
  body: { grade?: number | null; feedback?: string; hidden?: boolean; locked?: boolean; excluded?: boolean },
) {
  return request(`/api/v1/gradebook/items/${itemId}/grades/${userId}`, {
    method: "POST",
    token,
    body,
  });
}

// --- Groups ---

export type GroupMember = { user_id: string; name: string; email: string };

export type Group = {
  id: string;
  course_id: string;
  name: string;
  description: string;
  members: GroupMember[];
};

export type Grouping = {
  id: string;
  course_id: string;
  name: string;
  description: string;
  group_ids: string[];
};

export function listGroups(token: string, courseId: string) {
  return request<Group[]>(`/api/v1/courses/${courseId}/groups`, { token });
}

export function createGroup(token: string, courseId: string, name: string, description = "") {
  return request<Group>(`/api/v1/courses/${courseId}/groups`, {
    method: "POST",
    token,
    body: { name, description },
  });
}

export function autoCreateGroups(
  token: string,
  courseId: string,
  body: { count?: number; size?: number; prefix?: string },
) {
  return request<Group[]>(`/api/v1/courses/${courseId}/groups/auto`, {
    method: "POST",
    token,
    body,
  });
}

export function deleteGroup(token: string, groupId: string) {
  return request<void>(`/api/v1/groups/${groupId}`, { method: "DELETE", token });
}

export function addGroupMember(token: string, groupId: string, userId: string) {
  return request<void>(`/api/v1/groups/${groupId}/members`, {
    method: "POST",
    token,
    body: { user_id: userId },
  });
}

export function removeGroupMember(token: string, groupId: string, userId: string) {
  return request<void>(`/api/v1/groups/${groupId}/members/${userId}`, { method: "DELETE", token });
}

export function listGroupings(token: string, courseId: string) {
  return request<Grouping[]>(`/api/v1/courses/${courseId}/groupings`, { token });
}

export function createGrouping(token: string, courseId: string, name: string) {
  return request<Grouping>(`/api/v1/courses/${courseId}/groupings`, {
    method: "POST",
    token,
    body: { name },
  });
}

export function deleteGrouping(token: string, groupingId: string) {
  return request<void>(`/api/v1/groupings/${groupingId}`, { method: "DELETE", token });
}

export function addGroupToGrouping(token: string, groupingId: string, groupId: string) {
  return request<void>(`/api/v1/groupings/${groupingId}/groups`, {
    method: "POST",
    token,
    body: { group_id: groupId },
  });
}

export function removeGroupFromGrouping(token: string, groupingId: string, groupId: string) {
  return request<void>(`/api/v1/groupings/${groupingId}/groups/${groupId}`, {
    method: "DELETE",
    token,
  });
}

// --- Calendar ---

export type RepeatRule = "none" | "daily" | "weekly" | "monthly";

export type CalendarEvent = {
  id: string;
  event_type: "course" | "user" | "site";
  course_id: string | null;
  course_module_id: string | null;
  event_kind: string;
  name: string;
  description: string;
  start_at: string;
  end_at: string | null;
  repeat_rule: RepeatRule;
  repeat_until: string | null;
};

export type EventBody = {
  name: string;
  description?: string;
  start_at: string;
  end_at?: string;
  repeat_rule?: RepeatRule;
  repeat_until?: string;
};

export function listMyEvents(token: string, from: Date, to: Date) {
  const q = `from=${encodeURIComponent(from.toISOString())}&to=${encodeURIComponent(to.toISOString())}`;
  return request<CalendarEvent[]>(`/api/v1/users/me/events?${q}`, { token });
}

export function listCourseEvents(token: string, courseId: string, from: Date, to: Date) {
  const q = `from=${encodeURIComponent(from.toISOString())}&to=${encodeURIComponent(to.toISOString())}`;
  return request<CalendarEvent[]>(`/api/v1/courses/${courseId}/events?${q}`, { token });
}

export function createMyEvent(token: string, body: EventBody) {
  return request<CalendarEvent>("/api/v1/users/me/events", { method: "POST", token, body });
}

export function createCourseEvent(token: string, courseId: string, body: EventBody) {
  return request<CalendarEvent>(`/api/v1/courses/${courseId}/events`, {
    method: "POST",
    token,
    body,
  });
}

export function deleteEvent(token: string, eventId: string) {
  return request<void>(`/api/v1/events/${eventId}`, { method: "DELETE", token });
}
