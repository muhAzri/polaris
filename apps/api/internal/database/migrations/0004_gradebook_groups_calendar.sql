-- Gradebook, groups, and calendar. Any future activity module (assignment,
-- quiz, forum, ...) should write grades into this gradebook rather than
-- building its own grade storage.

-- --- Gradebook ---
-- No nested categories yet (Moodle supports them, but a flat list per
-- course is enough for now) — every course gets one default
-- "Uncategorized" category, same pattern as course_categories in 0002.
CREATE TABLE grade_categories (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    course_id   UUID NOT NULL REFERENCES courses(id) ON DELETE CASCADE,
    name        TEXT NOT NULL,
    aggregation TEXT NOT NULL DEFAULT 'mean' CHECK (aggregation IN ('mean', 'weighted')),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (course_id, name)
);
CREATE INDEX idx_grade_categories_course ON grade_categories(course_id);

-- course_module_id is set when a grade item belongs to an activity (a
-- future assign/quiz instance); NULL means a manually-added grade item.
CREATE TABLE grade_items (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    course_id        UUID NOT NULL REFERENCES courses(id) ON DELETE CASCADE,
    category_id      UUID NOT NULL REFERENCES grade_categories(id) ON DELETE CASCADE,
    course_module_id UUID NULL REFERENCES course_modules(id) ON DELETE CASCADE,
    name             TEXT NOT NULL,
    max_grade        NUMERIC NOT NULL DEFAULT 100,
    weight           NUMERIC NULL,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_grade_items_course ON grade_items(course_id);
CREATE INDEX idx_grade_items_category ON grade_items(category_id);

CREATE TABLE grade_grades (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    grade_item_id UUID NOT NULL REFERENCES grade_items(id) ON DELETE CASCADE,
    user_id       UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    grade         NUMERIC NULL,
    feedback      TEXT NOT NULL DEFAULT '',
    graded_by     UUID NULL REFERENCES users(id) ON DELETE SET NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (grade_item_id, user_id)
);
CREATE INDEX idx_grade_grades_user ON grade_grades(user_id);

INSERT INTO grade_categories (course_id, name, aggregation)
SELECT id, 'Uncategorized', 'mean' FROM courses;

-- --- Groups & groupings ---
CREATE TABLE groups (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    course_id   UUID NOT NULL REFERENCES courses(id) ON DELETE CASCADE,
    name        TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (course_id, name)
);
CREATE INDEX idx_groups_course ON groups(course_id);

CREATE TABLE group_members (
    group_id   UUID NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
    user_id    UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (group_id, user_id)
);

-- Groupings bundle groups together so a later module can restrict itself to
-- one grouping ("group mode"). No module reads these yet, so there's no API
-- for them beyond the schema existing to reference — same treatment
-- enrolment_methods gave the self/cohort/guest types it doesn't implement
-- yet in 0002.
CREATE TABLE groupings (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    course_id   UUID NOT NULL REFERENCES courses(id) ON DELETE CASCADE,
    name        TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (course_id, name)
);

CREATE TABLE groupings_groups (
    grouping_id UUID NOT NULL REFERENCES groupings(id) ON DELETE CASCADE,
    group_id    UUID NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
    PRIMARY KEY (grouping_id, group_id)
);

-- --- Calendar ---
-- A course event belongs to a course, a user event is personal, a site
-- event is global — exactly one of course_id/user_id is set, matching
-- event_type.
CREATE TABLE calendar_events (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    event_type  TEXT NOT NULL CHECK (event_type IN ('course', 'user', 'site')),
    course_id   UUID NULL REFERENCES courses(id) ON DELETE CASCADE,
    user_id     UUID NULL REFERENCES users(id) ON DELETE CASCADE,
    name        TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    start_at    TIMESTAMPTZ NOT NULL,
    end_at      TIMESTAMPTZ NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (
        (event_type = 'course' AND course_id IS NOT NULL AND user_id IS NULL) OR
        (event_type = 'user' AND user_id IS NOT NULL AND course_id IS NULL) OR
        (event_type = 'site' AND course_id IS NULL AND user_id IS NULL)
    )
);
CREATE INDEX idx_calendar_events_course ON calendar_events(course_id);
CREATE INDEX idx_calendar_events_user ON calendar_events(user_id);
CREATE INDEX idx_calendar_events_start ON calendar_events(start_at);

-- --- New capabilities ---
-- grade:view/grade:manage/grade:viewhidden were already seeded in 0002,
-- ahead of the gradebook actually existing, so no new grade capability is
-- needed here. Course-scoped calendar events reuse course:manage; only
-- site-wide events need a new capability, kept admin-only.
INSERT INTO capabilities (name, description) VALUES
    ('group:manage', 'Create groups and manage group membership'),
    ('calendar:manage', 'Create and edit site-wide calendar events');

INSERT INTO role_capabilities (role_id, capability_id)
SELECT r.id, c.id FROM roles r, capabilities c
WHERE (r.name = 'teacher' AND c.name = 'group:manage')
   OR (r.name = 'admin' AND c.name IN ('group:manage', 'calendar:manage'));
