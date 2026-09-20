-- Course access is now membership-based, as in Moodle: a user reaches a
-- course only through an enrolment (or as an admin). Enrolment is manual by
-- default; self enrolment exists per course but starts disabled.

INSERT INTO enrolment_methods (course_id, type, enabled)
SELECT id, 'self', false FROM courses
ON CONFLICT (course_id, type) DO NOTHING;

-- A course owner is enrolled in their own course so "my courses" and access
-- checks need no special case for owners.
INSERT INTO enrollments (course_id, user_id, enrolment_method_id)
SELECT c.id, c.owner_id, em.id
FROM courses c
JOIN enrolment_methods em ON em.course_id = c.id AND em.type = 'manual'
ON CONFLICT (course_id, user_id) DO NOTHING;
