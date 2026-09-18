-- Foundational schema every feature module builds on: role/capability/
-- context RBAC, generic course structure (category/section/module),
-- enrolment methods framework, and the event_log backing the in-process
-- event bus.

CREATE TABLE roles (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name        TEXT NOT NULL UNIQUE,
    description TEXT NOT NULL DEFAULT '',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE capabilities (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name        TEXT NOT NULL UNIQUE,
    description TEXT NOT NULL DEFAULT '',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE role_capabilities (
    role_id       UUID NOT NULL REFERENCES roles(id) ON DELETE CASCADE,
    capability_id UUID NOT NULL REFERENCES capabilities(id) ON DELETE CASCADE,
    PRIMARY KEY (role_id, capability_id)
);

-- Mirrors Moodle's context levels, simplified to the levels currently used.
CREATE TABLE contexts (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    level       TEXT NOT NULL CHECK (level IN ('system', 'category', 'course', 'module', 'user')),
    instance_id UUID NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX idx_contexts_system ON contexts(level) WHERE level = 'system';
CREATE UNIQUE INDEX idx_contexts_instance ON contexts(level, instance_id) WHERE instance_id IS NOT NULL;

CREATE TABLE role_assignments (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    role_id    UUID NOT NULL REFERENCES roles(id) ON DELETE CASCADE,
    user_id    UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    context_id UUID NOT NULL REFERENCES contexts(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (role_id, user_id, context_id)
);
CREATE INDEX idx_role_assignments_user ON role_assignments(user_id);
CREATE INDEX idx_role_assignments_context ON role_assignments(context_id);

CREATE TABLE course_categories (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    parent_id  UUID NULL REFERENCES course_categories(id) ON DELETE SET NULL,
    name       TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

ALTER TABLE courses
    ADD COLUMN category_id UUID NULL REFERENCES course_categories(id) ON DELETE SET NULL;

CREATE TABLE course_sections (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    course_id  UUID NOT NULL REFERENCES courses(id) ON DELETE CASCADE,
    title      TEXT NOT NULL DEFAULT '',
    position   INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (course_id, position)
);
CREATE INDEX idx_course_sections_course ON course_sections(course_id);

-- Polymorphic: module_type + instance_id point at a <module_type>-specific
-- table (e.g. mod_labels, mod_pages, or a future assignments/quizzes table)
-- owned by that module's own package, so course/section code never needs to
-- know about every module type that exists.
CREATE TABLE course_modules (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    section_id  UUID NOT NULL REFERENCES course_sections(id) ON DELETE CASCADE,
    module_type TEXT NOT NULL,
    instance_id UUID NOT NULL,
    position    INTEGER NOT NULL DEFAULT 0,
    visible     BOOLEAN NOT NULL DEFAULT true,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_course_modules_section ON course_modules(section_id);

CREATE TABLE enrolment_methods (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    course_id  UUID NOT NULL REFERENCES courses(id) ON DELETE CASCADE,
    type       TEXT NOT NULL CHECK (type IN ('manual', 'self', 'cohort', 'guest')),
    enabled    BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (course_id, type)
);

ALTER TABLE enrollments
    ADD COLUMN enrolment_method_id UUID NULL REFERENCES enrolment_methods(id) ON DELETE SET NULL;

CREATE TABLE event_log (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name       TEXT NOT NULL,
    context_id UUID NULL REFERENCES contexts(id) ON DELETE SET NULL,
    user_id    UUID NULL REFERENCES users(id) ON DELETE SET NULL,
    data       JSONB NOT NULL DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_event_log_name ON event_log(name);
CREATE INDEX idx_event_log_created_at ON event_log(created_at);

-- Seed roles matching the existing users.role enum, plus an initial
-- capability list (deliberately small and specific to what's implemented
-- today; grow it incrementally as features need new checks instead of
-- front-loading all of Moodle's 400+ capabilities).
INSERT INTO roles (name, description) VALUES
    ('student', 'Can view and participate in enrolled courses'),
    ('teacher', 'Can manage courses and grade students'),
    ('admin', 'Full system access');

INSERT INTO capabilities (name, description) VALUES
    ('course:view', 'View course content'),
    ('course:manage', 'Create and edit courses'),
    ('course:enrol', 'Enrol into a course'),
    ('mod:view', 'View activity modules'),
    ('mod:manage', 'Create and edit activity modules'),
    ('grade:view', 'View own grades'),
    ('grade:manage', 'Enter and edit grades'),
    ('grade:viewhidden', 'View hidden/unreleased grades');

INSERT INTO role_capabilities (role_id, capability_id)
SELECT r.id, c.id FROM roles r, capabilities c
WHERE (r.name = 'student' AND c.name IN ('course:view', 'course:enrol', 'mod:view', 'grade:view'))
   OR (r.name = 'teacher' AND c.name IN ('course:view', 'course:manage', 'course:enrol', 'mod:view', 'mod:manage', 'grade:view', 'grade:manage', 'grade:viewhidden'))
   OR (r.name = 'admin');

-- Singleton system context, and a default category so existing courses
-- have somewhere to live without a per-row backfill decision.
INSERT INTO contexts (level, instance_id) VALUES ('system', NULL);

INSERT INTO course_categories (name) VALUES ('Uncategorized');

UPDATE courses SET category_id = (SELECT id FROM course_categories WHERE name = 'Uncategorized')
WHERE category_id IS NULL;

-- Backfill: role assignments for users created before RBAC existed, and a
-- default section + manual enrolment method for courses created before the
-- course-structure/enrolment-methods tables existed.
INSERT INTO role_assignments (role_id, user_id, context_id)
SELECT r.id, u.id, (SELECT id FROM contexts WHERE level = 'system')
FROM users u
JOIN roles r ON r.name = u.role;

INSERT INTO course_sections (course_id, title, position)
SELECT id, 'General', 0 FROM courses;

INSERT INTO enrolment_methods (course_id, type)
SELECT id, 'manual' FROM courses;

UPDATE enrollments e
SET enrolment_method_id = em.id
FROM enrolment_methods em
WHERE em.course_id = e.course_id AND em.type = 'manual' AND e.enrolment_method_id IS NULL;

-- users.role stays the source of truth for register/login so existing
-- auth code doesn't need to change — this trigger keeps role_assignments
-- in sync with it automatically so nothing downstream has to remember to
-- write both.
CREATE OR REPLACE FUNCTION sync_user_role_assignment() RETURNS TRIGGER AS $$
DECLARE
    sys_context_id UUID;
    new_role_id UUID;
BEGIN
    SELECT id INTO sys_context_id FROM contexts WHERE level = 'system';
    SELECT id INTO new_role_id FROM roles WHERE name = NEW.role;

    IF TG_OP = 'UPDATE' AND OLD.role IS DISTINCT FROM NEW.role THEN
        DELETE FROM role_assignments
        WHERE user_id = NEW.id AND context_id = sys_context_id
          AND role_id IN (SELECT id FROM roles WHERE name = OLD.role);
    END IF;

    INSERT INTO role_assignments (role_id, user_id, context_id)
    VALUES (new_role_id, NEW.id, sys_context_id)
    ON CONFLICT (role_id, user_id, context_id) DO NOTHING;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_sync_user_role_assignment
AFTER INSERT OR UPDATE OF role ON users
FOR EACH ROW EXECUTE FUNCTION sync_user_role_assignment();
