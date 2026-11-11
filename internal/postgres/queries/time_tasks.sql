-- Time entries and task lists.

-- name: ListTimeEntries :many
SELECT sqlc.embed(e), count(*) OVER () AS total
FROM time_entries e
WHERE e.administration_id = @administration_id
  AND (@include_active::boolean OR e.ended_at IS NOT NULL)
  AND (sqlc.narg(from_day)::date IS NULL OR (e.started_at AT TIME ZONE @time_zone::text)::date >= sqlc.narg(from_day)::date)
  AND (sqlc.narg(to_day)::date IS NULL OR (e.started_at AT TIME ZONE @time_zone::text)::date <= sqlc.narg(to_day)::date)
  AND (sqlc.narg(on_day)::date IS NULL OR (e.started_at AT TIME ZONE @time_zone::text)::date = sqlc.narg(on_day)::date)
  AND (sqlc.narg(contact_id)::bigint IS NULL OR e.contact_id = sqlc.narg(contact_id)::bigint OR (@include_nil_contacts::boolean AND e.contact_id IS NULL))
  AND (sqlc.narg(project_id)::bigint IS NULL OR e.project_id = sqlc.narg(project_id)::bigint)
  AND (sqlc.narg(user_id)::bigint IS NULL OR e.user_id = sqlc.narg(user_id)::bigint)
  AND (@search::text = '' OR e.description ILIKE '%' || @search::text || '%')
  AND (cardinality(@states::text[]) = 0
       OR ('open' = ANY(@states::text[]) AND e.billable AND e.sales_invoice_id IS NULL)
       OR ('non_billable' = ANY(@states::text[]) AND NOT e.billable))
ORDER BY e.started_at DESC, e.id DESC
LIMIT sqlc.narg(max_rows) OFFSET @skip;

-- name: TimeEntryByID :one
SELECT * FROM time_entries WHERE administration_id = $1 AND id = $2;

-- name: RunningTimeEntries :many
SELECT * FROM time_entries WHERE administration_id = $1 AND user_id = $2 AND ended_at IS NULL;

-- name: TimeEntriesOfInvoices :many
SELECT * FROM time_entries WHERE administration_id = @administration_id AND sales_invoice_id = ANY(@ids::bigint[])
ORDER BY started_at, id;

-- name: UpsertTimeEntry :exec
INSERT INTO time_entries (id, administration_id, user_id, contact_id, project_id, sales_invoice_id, started_at, ended_at, description,
    paused_duration, billable, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
ON CONFLICT (id) DO UPDATE SET user_id = EXCLUDED.user_id, contact_id = EXCLUDED.contact_id, project_id = EXCLUDED.project_id,
    sales_invoice_id = EXCLUDED.sales_invoice_id, started_at = EXCLUDED.started_at, ended_at = EXCLUDED.ended_at,
    description = EXCLUDED.description, paused_duration = EXCLUDED.paused_duration, billable = EXCLUDED.billable, updated_at = EXCLUDED.updated_at;

-- name: DeleteTimeEntry :exec
DELETE FROM time_entries WHERE administration_id = $1 AND id = $2;

-- name: TimeEntryUsers :many
SELECT u.id, u.name, m.created_at, m.updated_at
FROM administration_members m JOIN users u ON u.id = m.user_id
WHERE m.administration_id = @administration_id AND u.id = ANY(@ids::bigint[]);

-- name: ContactExists :one
SELECT EXISTS (SELECT 1 FROM contacts WHERE administration_id = $1 AND id = $2);

-- name: ProjectExists :one
SELECT EXISTS (SELECT 1 FROM projects WHERE administration_id = $1 AND id = $2);

-- name: SalesInvoiceExists :one
SELECT EXISTS (SELECT 1 FROM sales_invoices WHERE administration_id = $1 AND id = $2);

-- name: ListTaskListTemplates :many
SELECT * FROM task_list_templates WHERE administration_id = $1 ORDER BY lower(name), id;

-- name: TaskListTemplateByID :one
SELECT * FROM task_list_templates WHERE administration_id = $1 AND id = $2;

-- name: UpsertTaskListTemplate :exec
INSERT INTO task_list_templates (id, administration_id, name, description, period, deadline_after, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
ON CONFLICT (id) DO UPDATE SET name = EXCLUDED.name, description = EXCLUDED.description, period = EXCLUDED.period,
    deadline_after = EXCLUDED.deadline_after, updated_at = EXCLUDED.updated_at;

-- name: DeleteTaskListTemplate :exec
DELETE FROM task_list_templates WHERE administration_id = $1 AND id = $2;

-- name: ListTaskLists :many
SELECT sqlc.embed(l), count(*) OVER () AS total
FROM task_lists l
WHERE l.administration_id = @administration_id
ORDER BY l.created_at DESC, l.id DESC
LIMIT sqlc.narg(max_rows) OFFSET @skip;

-- name: TaskListByID :one
SELECT * FROM task_lists WHERE administration_id = $1 AND id = $2;

-- name: UpsertTaskList :exec
INSERT INTO task_lists (id, administration_id, template_id, name, description, deadline_on, period_begin, period_end, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
ON CONFLICT (id) DO UPDATE SET name = EXCLUDED.name, description = EXCLUDED.description, deadline_on = EXCLUDED.deadline_on,
    period_begin = EXCLUDED.period_begin, period_end = EXCLUDED.period_end, updated_at = EXCLUDED.updated_at;

-- name: DeleteTaskList :exec
DELETE FROM task_lists WHERE administration_id = $1 AND id = $2;

-- name: TaskListGroupByID :one
SELECT * FROM task_list_groups WHERE administration_id = $1 AND id = $2;

-- name: TaskListGroupsOf :many
SELECT * FROM task_list_groups
WHERE administration_id = @administration_id AND (task_list_id = ANY(@owners::bigint[]) OR template_id = ANY(@owners::bigint[]))
ORDER BY row_order, created_at, id;

-- name: UpsertTaskListGroup :exec
INSERT INTO task_list_groups (id, administration_id, task_list_id, template_id, name, is_first, row_order, report_type, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
ON CONFLICT (id) DO UPDATE SET name = EXCLUDED.name, row_order = EXCLUDED.row_order, report_type = EXCLUDED.report_type, updated_at = EXCLUDED.updated_at;

-- name: DeleteTaskListGroup :exec
DELETE FROM task_list_groups WHERE administration_id = $1 AND id = $2;

-- name: TaskListTaskByID :one
SELECT * FROM task_list_tasks WHERE administration_id = $1 AND id = $2;

-- name: TaskListTasksOf :many
SELECT * FROM task_list_tasks WHERE administration_id = @administration_id AND group_id = ANY(@groups::bigint[])
ORDER BY row_order, created_at, id;

-- name: UpsertTaskListTask :exec
INSERT INTO task_list_tasks (id, administration_id, group_id, parent_task_id, name, description, row_order, completed_at, report_type,
    assignee_id, related_entity_id, generated, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
ON CONFLICT (id) DO UPDATE SET name = EXCLUDED.name, description = EXCLUDED.description, row_order = EXCLUDED.row_order,
    completed_at = EXCLUDED.completed_at, report_type = EXCLUDED.report_type, assignee_id = EXCLUDED.assignee_id,
    related_entity_id = EXCLUDED.related_entity_id, updated_at = EXCLUDED.updated_at;

-- name: DeleteTaskListTask :exec
DELETE FROM task_list_tasks WHERE administration_id = $1 AND id = $2;

-- name: DeleteTaskListGroupTasks :exec
DELETE FROM task_list_tasks WHERE administration_id = $1 AND group_id = $2;

-- name: DeleteGeneratedSubtasks :exec
DELETE FROM task_list_tasks WHERE administration_id = $1 AND parent_task_id = $2 AND generated;

-- name: ContactHasTimeEntries :one
SELECT EXISTS (SELECT 1 FROM time_entries WHERE administration_id = @administration_id AND contact_id = @contact_id);

-- name: ProjectHasTimeEntries :one
SELECT EXISTS (SELECT 1 FROM time_entries WHERE administration_id = @administration_id AND project_id = @project_id);
