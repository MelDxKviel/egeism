-- name: CreateClass :one
INSERT INTO classes (teacher_id, name) VALUES ($1, $2) RETURNING *;

-- name: GetClass :one
SELECT * FROM classes WHERE id = $1;

-- name: RenameClass :one
UPDATE classes SET name = $2 WHERE id = $1 RETURNING *;

-- name: DeleteClass :execrows
DELETE FROM classes WHERE id = $1;

-- name: ListClassesForTeacher :many
SELECT c.*, count(cm.id) AS member_count
FROM classes c
LEFT JOIN class_members cm ON cm.class_id = c.id
WHERE c.teacher_id = $1
GROUP BY c.id
ORDER BY c.name;

-- name: ListAllClasses :many
-- Admin view: every class with its teacher's name.
SELECT c.*, u.name AS teacher_name, count(cm.id) AS member_count
FROM classes c
JOIN users u ON u.id = c.teacher_id
LEFT JOIN class_members cm ON cm.class_id = c.id
GROUP BY c.id, u.name
ORDER BY u.name, c.name;

-- name: AddClassMember :exec
INSERT INTO class_members (class_id, student_id) VALUES ($1, $2)
ON CONFLICT (class_id, student_id) DO NOTHING;

-- name: RemoveClassMember :execrows
DELETE FROM class_members WHERE class_id = $1 AND student_id = $2;

-- name: ListClassMembers :many
SELECT u.* FROM users u
JOIN class_members cm ON cm.student_id = u.id
WHERE cm.class_id = $1
ORDER BY u.name;

-- name: ListClassesForStudent :many
-- The student's profile: which classes (and whose) they belong to.
SELECT c.*, u.name AS teacher_name
FROM classes c
JOIN class_members cm ON cm.class_id = c.id
JOIN users u ON u.id = c.teacher_id
WHERE cm.student_id = $1
ORDER BY c.name;

-- name: ListClassMembershipsForTeacher :many
-- All (student, class) pairs across one teacher's classes, to tag the teacher's
-- student list with class names in one query.
SELECT cm.student_id, c.id AS class_id, c.name AS class_name
FROM class_members cm
JOIN classes c ON c.id = cm.class_id
WHERE c.teacher_id = $1
ORDER BY c.name;

-- name: ClassMastery :many
-- The class overview grid: per-member per-number success for one subject.
SELECT att.student_id, t.number,
       count(*)                             AS total,
       count(*) FILTER (WHERE a.is_correct) AS correct
FROM answers a
JOIN attempts att ON att.id = a.attempt_id
JOIN tasks t      ON t.id = a.task_id
WHERE t.subject_id = $2
  AND att.student_id IN (SELECT student_id FROM class_members WHERE class_id = $1)
GROUP BY att.student_id, t.number
ORDER BY att.student_id, t.number;
