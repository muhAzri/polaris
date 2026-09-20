-- Brings the foundation, content, gradebook, groups and calendar schema
-- closer to how Moodle actually models them:
--   * roles are assigned per context (site, category, course, module) and
--     permissions are resolved by walking the context path, with
--     allow/prevent/prohibit and per-context overrides
--   * enrolment carries a status, a time window and the role it grants
--   * courses, categories, sections and modules gain the fields Moodle has
--   * the gradebook becomes a tree of categories with real aggregation
--   * groups get groupings, group mode and module-linked calendar events

-- =====================================================================
-- RBAC: archetypes, permissions, overrides, context tree
-- =====================================================================

ALTER TABLE roles
    ADD COLUMN display_name TEXT NOT NULL DEFAULT '',
    ADD COLUMN archetype    TEXT NOT NULL DEFAULT '',
    ADD COLUMN is_system    BOOLEAN NOT NULL DEFAULT false;

UPDATE roles SET display_name = 'Student', archetype = 'student', is_system = true WHERE name = 'student';
UPDATE roles SET display_name = 'Teacher', archetype = 'editingteacher', is_system = true WHERE name = 'teacher';
UPDATE roles SET display_name = 'Site administrator', archetype = '', is_system = true WHERE name = 'admin';

INSERT INTO roles (name, display_name, description, archetype, is_system) VALUES
    ('nonediting_teacher', 'Non-editing teacher', 'Can view and grade students but cannot change course content', 'teacher', true),
    ('manager', 'Manager', 'Can manage courses, categories and roles', 'manager', true),
    ('coursecreator', 'Course creator', 'Can create new courses', 'coursecreator', true),
    ('user', 'Authenticated user', 'Implicitly held by every signed-in user', 'user', true);

-- A capability can be allowed, prevented (not granted here, but a broader
-- context may still grant it) or prohibited (cannot be overridden further
-- down the context tree).
ALTER TABLE role_capabilities
    ADD COLUMN permission TEXT NOT NULL DEFAULT 'allow'
        CHECK (permission IN ('allow', 'prevent', 'prohibit'));

CREATE TABLE role_overrides (
    role_id       UUID NOT NULL REFERENCES roles(id) ON DELETE CASCADE,
    capability_id UUID NOT NULL REFERENCES capabilities(id) ON DELETE CASCADE,
    context_id    UUID NOT NULL REFERENCES contexts(id) ON DELETE CASCADE,
    permission    TEXT NOT NULL CHECK (permission IN ('allow', 'prevent', 'prohibit')),
    PRIMARY KEY (role_id, capability_id, context_id)
);
CREATE INDEX idx_role_overrides_context ON role_overrides(context_id);

ALTER TABLE contexts
    ADD COLUMN parent_id UUID NULL REFERENCES contexts(id) ON DELETE SET NULL;
CREATE INDEX idx_contexts_parent ON contexts(parent_id);

-- Existing contexts default to hanging off the system context; the API
-- refreshes each one's real parent (category, course) the next time it
-- resolves it.
UPDATE contexts SET parent_id = (SELECT id FROM contexts WHERE level = 'system')
WHERE level <> 'system' AND parent_id IS NULL;

DELETE FROM capabilities WHERE name = 'course:manage';

INSERT INTO capabilities (name, description) VALUES
    ('course:create', 'Create new courses'),
    ('course:update', 'Change course settings and sections'),
    ('course:delete', 'Delete courses'),
    ('course:viewhidden', 'See hidden courses'),
    ('course:viewparticipants', 'See the participant list'),
    ('enrol:manage', 'Enrol and unenrol users and configure enrolment methods'),
    ('role:assign', 'Assign roles to users in a context'),
    ('role:override', 'Override role permissions in a context'),
    ('role:manage', 'Define roles and their default permissions'),
    ('category:manage', 'Create, edit and delete course categories'),
    ('mod:viewhidden', 'See hidden activities and sections'),
    ('grade:viewall', 'See every student''s grades'),
    ('grade:edit', 'Enter and override grades'),
    ('calendar:manageentries', 'Create and edit course calendar events');

-- Reset the grant matrix now that the capability list is settled. Admins
-- pass every check in code, so they need no rows here.
DELETE FROM role_capabilities;

INSERT INTO role_capabilities (role_id, capability_id)
SELECT r.id, c.id
FROM (VALUES
    ('student', 'mod:view'),
    ('student', 'grade:view'),

    ('nonediting_teacher', 'mod:view'),
    ('nonediting_teacher', 'mod:viewhidden'),
    ('nonediting_teacher', 'course:viewhidden'),
    ('nonediting_teacher', 'course:viewparticipants'),
    ('nonediting_teacher', 'grade:view'),
    ('nonediting_teacher', 'grade:viewall'),
    ('nonediting_teacher', 'grade:viewhidden'),
    ('nonediting_teacher', 'grade:edit'),

    ('teacher', 'mod:view'),
    ('teacher', 'mod:manage'),
    ('teacher', 'mod:viewhidden'),
    ('teacher', 'course:update'),
    ('teacher', 'course:viewhidden'),
    ('teacher', 'course:viewparticipants'),
    ('teacher', 'enrol:manage'),
    ('teacher', 'role:assign'),
    ('teacher', 'grade:view'),
    ('teacher', 'grade:viewall'),
    ('teacher', 'grade:viewhidden'),
    ('teacher', 'grade:edit'),
    ('teacher', 'grade:manage'),
    ('teacher', 'group:manage'),
    ('teacher', 'calendar:manageentries'),

    ('manager', 'course:view'),
    ('manager', 'course:create'),
    ('manager', 'course:update'),
    ('manager', 'course:delete'),
    ('manager', 'course:viewhidden'),
    ('manager', 'course:viewparticipants'),
    ('manager', 'enrol:manage'),
    ('manager', 'role:assign'),
    ('manager', 'role:override'),
    ('manager', 'category:manage'),
    ('manager', 'mod:view'),
    ('manager', 'mod:manage'),
    ('manager', 'mod:viewhidden'),
    ('manager', 'grade:view'),
    ('manager', 'grade:viewall'),
    ('manager', 'grade:viewhidden'),
    ('manager', 'grade:edit'),
    ('manager', 'grade:manage'),
    ('manager', 'group:manage'),
    ('manager', 'calendar:manageentries'),

    ('coursecreator', 'course:create'),

    ('user', 'course:enrol')
) AS v(role_name, capability_name)
JOIN roles r ON r.name = v.role_name
JOIN capabilities c ON c.name = v.capability_name;

-- users.role stays the sign-up/login flag, but it now only maps to
-- site-level roles: admins become site administrators, teachers become
-- course creators. Being a teacher or student *of a course* comes from a
-- role assignment at that course's context, created by enrolment.
DELETE FROM role_assignments
WHERE context_id = (SELECT id FROM contexts WHERE level = 'system')
  AND role_id IN (SELECT id FROM roles WHERE name IN ('student', 'teacher'));

INSERT INTO role_assignments (role_id, user_id, context_id)
SELECT r.id, u.id, (SELECT id FROM contexts WHERE level = 'system')
FROM users u
JOIN roles r ON r.name = 'coursecreator'
WHERE u.role = 'teacher'
ON CONFLICT DO NOTHING;

CREATE OR REPLACE FUNCTION sync_user_role_assignment() RETURNS TRIGGER AS $$
DECLARE
    sys_context_id UUID;
    site_role_id UUID;
BEGIN
    SELECT id INTO sys_context_id FROM contexts WHERE level = 'system';

    DELETE FROM role_assignments
    WHERE user_id = NEW.id AND context_id = sys_context_id
      AND role_id IN (SELECT id FROM roles WHERE name IN ('admin', 'coursecreator', 'student', 'teacher'));

    SELECT id INTO site_role_id FROM roles
    WHERE name = CASE NEW.role WHEN 'admin' THEN 'admin' WHEN 'teacher' THEN 'coursecreator' ELSE NULL END;

    IF site_role_id IS NOT NULL THEN
        INSERT INTO role_assignments (role_id, user_id, context_id)
        VALUES (site_role_id, NEW.id, sys_context_id)
        ON CONFLICT (role_id, user_id, context_id) DO NOTHING;
    END IF;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- =====================================================================
-- Courses: the fields Moodle keeps on a course
-- =====================================================================

ALTER TABLE courses
    ADD COLUMN short_name TEXT NOT NULL DEFAULT '',
    ADD COLUMN id_number  TEXT NOT NULL DEFAULT '',
    ADD COLUMN format     TEXT NOT NULL DEFAULT 'topics' CHECK (format IN ('topics', 'weeks')),
    ADD COLUMN start_date TIMESTAMPTZ NULL,
    ADD COLUMN end_date   TIMESTAMPTZ NULL,
    ADD COLUMN visible    BOOLEAN NOT NULL DEFAULT true;

UPDATE courses
SET short_name = trim(both '-' from lower(regexp_replace(left(title, 24), '[^a-zA-Z0-9]+', '-', 'g'))) || '-' || left(id::text, 4);

CREATE UNIQUE INDEX idx_courses_short_name ON courses(lower(short_name));
CREATE UNIQUE INDEX idx_courses_id_number ON courses(id_number) WHERE id_number <> '';

ALTER TABLE course_categories
    ADD COLUMN description TEXT NOT NULL DEFAULT '',
    ADD COLUMN id_number   TEXT NOT NULL DEFAULT '',
    ADD COLUMN visible     BOOLEAN NOT NULL DEFAULT true,
    ADD COLUMN is_default  BOOLEAN NOT NULL DEFAULT false,
    ADD COLUMN sort_order  INTEGER NOT NULL DEFAULT 0;

UPDATE course_categories SET is_default = true WHERE name = 'Uncategorized';
CREATE UNIQUE INDEX idx_course_categories_default ON course_categories(is_default) WHERE is_default;

ALTER TABLE course_sections
    ADD COLUMN summary TEXT NOT NULL DEFAULT '',
    ADD COLUMN visible BOOLEAN NOT NULL DEFAULT true;

-- Reordering swaps positions inside one transaction, so the uniqueness
-- check has to wait until commit.
ALTER TABLE course_sections DROP CONSTRAINT course_sections_course_id_position_key;
ALTER TABLE course_sections
    ADD CONSTRAINT course_sections_course_id_position_key UNIQUE (course_id, position) DEFERRABLE INITIALLY DEFERRED;

-- =====================================================================
-- Course modules: intro, group mode, availability window
-- =====================================================================

ALTER TABLE course_modules
    ADD COLUMN intro           TEXT NOT NULL DEFAULT '',
    ADD COLUMN group_mode      TEXT NOT NULL DEFAULT 'none' CHECK (group_mode IN ('none', 'separate', 'visible')),
    ADD COLUMN grouping_id     UUID NULL REFERENCES groupings(id) ON DELETE SET NULL,
    ADD COLUMN available_from  TIMESTAMPTZ NULL,
    ADD COLUMN available_until TIMESTAMPTZ NULL,
    ADD COLUMN updated_at      TIMESTAMPTZ NOT NULL DEFAULT now();

ALTER TABLE mod_book_chapters
    ADD COLUMN subchapter BOOLEAN NOT NULL DEFAULT false,
    ADD COLUMN hidden     BOOLEAN NOT NULL DEFAULT false;

ALTER TABLE mod_folder_files
    ADD COLUMN dir_path TEXT NOT NULL DEFAULT '/';

-- =====================================================================
-- Enrolment: status, time window, role, and self-enrolment settings
-- =====================================================================

ALTER TABLE enrollments
    ADD COLUMN status     TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'suspended')),
    ADD COLUMN time_start TIMESTAMPTZ NULL,
    ADD COLUMN time_end   TIMESTAMPTZ NULL,
    ADD COLUMN updated_at TIMESTAMPTZ NOT NULL DEFAULT now();

ALTER TABLE enrolment_methods
    ADD COLUMN role_id       UUID NULL REFERENCES roles(id) ON DELETE SET NULL,
    ADD COLUMN enrol_key     TEXT NOT NULL DEFAULT '',
    ADD COLUMN max_users     INTEGER NULL,
    ADD COLUMN enrol_start   TIMESTAMPTZ NULL,
    ADD COLUMN enrol_end     TIMESTAMPTZ NULL,
    ADD COLUMN duration_days INTEGER NULL;

UPDATE enrolment_methods SET role_id = (SELECT id FROM roles WHERE name = 'student');

-- Course contexts used to be created lazily; role assignments need them
-- now, so create one for every existing course.
INSERT INTO contexts (level, instance_id, parent_id)
SELECT 'course', c.id, (SELECT id FROM contexts WHERE level = 'system')
FROM courses c
ON CONFLICT (level, instance_id) WHERE instance_id IS NOT NULL DO NOTHING;

-- Existing enrolments become course-level role assignments: the owner and
-- any teacher-flagged user get the teacher role, everyone else student.
INSERT INTO role_assignments (role_id, user_id, context_id)
SELECT r.id, e.user_id, cc.id
FROM enrollments e
JOIN courses c ON c.id = e.course_id
JOIN users u ON u.id = e.user_id
JOIN contexts cc ON cc.level = 'course' AND cc.instance_id = c.id
JOIN roles r ON r.name = CASE WHEN c.owner_id = e.user_id OR u.role = 'teacher' THEN 'teacher' ELSE 'student' END
ON CONFLICT DO NOTHING;

-- =====================================================================
-- Gradebook: category tree, aggregation methods, hidden/locked grades
-- =====================================================================

ALTER TABLE grade_categories
    ADD COLUMN parent_id    UUID NULL REFERENCES grade_categories(id) ON DELETE CASCADE,
    ADD COLUMN is_root      BOOLEAN NOT NULL DEFAULT false,
    ADD COLUMN max_grade    NUMERIC NOT NULL DEFAULT 100,
    ADD COLUMN weight       NUMERIC NULL,
    ADD COLUMN drop_lowest  INTEGER NOT NULL DEFAULT 0 CHECK (drop_lowest >= 0),
    ADD COLUMN hidden       BOOLEAN NOT NULL DEFAULT false,
    ADD COLUMN sort_order   INTEGER NOT NULL DEFAULT 0;

ALTER TABLE grade_categories DROP CONSTRAINT grade_categories_aggregation_check;
ALTER TABLE grade_categories
    ADD CONSTRAINT grade_categories_aggregation_check
    CHECK (aggregation IN ('mean', 'weighted', 'natural', 'min', 'max', 'median', 'mode'));

-- The old flat default category becomes the course-level root, which is
-- Moodle's "course total"; every other category now hangs beneath it.
UPDATE grade_categories SET is_root = true, name = 'Course total' WHERE name = 'Uncategorized';
UPDATE grade_categories gc SET parent_id = root.id
FROM grade_categories root
WHERE root.course_id = gc.course_id AND root.is_root AND NOT gc.is_root;
CREATE UNIQUE INDEX idx_grade_categories_root ON grade_categories(course_id) WHERE is_root;

ALTER TABLE grade_items
    ADD COLUMN min_grade   NUMERIC NOT NULL DEFAULT 0,
    ADD COLUMN pass_grade  NUMERIC NULL,
    ADD COLUMN hidden      BOOLEAN NOT NULL DEFAULT false,
    ADD COLUMN sort_order  INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN updated_at  TIMESTAMPTZ NOT NULL DEFAULT now();

ALTER TABLE grade_grades
    ADD COLUMN hidden     BOOLEAN NOT NULL DEFAULT false,
    ADD COLUMN locked     BOOLEAN NOT NULL DEFAULT false,
    ADD COLUMN overridden BOOLEAN NOT NULL DEFAULT false,
    ADD COLUMN excluded   BOOLEAN NOT NULL DEFAULT false;

CREATE TABLE grade_history (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    grade_item_id UUID NOT NULL REFERENCES grade_items(id) ON DELETE CASCADE,
    user_id       UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    old_grade     NUMERIC NULL,
    new_grade     NUMERIC NULL,
    feedback      TEXT NOT NULL DEFAULT '',
    changed_by    UUID NULL REFERENCES users(id) ON DELETE SET NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_grade_history_item_user ON grade_history(grade_item_id, user_id);

-- =====================================================================
-- Calendar: repeating events and events owned by an activity
-- =====================================================================

ALTER TABLE calendar_events
    ADD COLUMN repeat_rule      TEXT NOT NULL DEFAULT 'none' CHECK (repeat_rule IN ('none', 'daily', 'weekly', 'monthly')),
    ADD COLUMN repeat_until     TIMESTAMPTZ NULL,
    ADD COLUMN course_module_id UUID NULL REFERENCES course_modules(id) ON DELETE CASCADE,
    ADD COLUMN event_kind       TEXT NOT NULL DEFAULT 'standard',
    ADD COLUMN updated_at       TIMESTAMPTZ NOT NULL DEFAULT now();
CREATE INDEX idx_calendar_events_module ON calendar_events(course_module_id);

CREATE INDEX idx_groupings_groups_group ON groupings_groups(group_id);
