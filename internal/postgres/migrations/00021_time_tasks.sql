-- +goose Up
-- time_entries record time members spent, for a contact and a project, and
-- can be billed on a sales invoice. One without ended_at is a running timer.
CREATE TABLE time_entries (
    id                bigint PRIMARY KEY,
    administration_id bigint  NOT NULL REFERENCES administrations (id) ON DELETE CASCADE,
    user_id           bigint  NOT NULL REFERENCES users (id),
    contact_id        bigint  REFERENCES contacts (id),
    project_id        bigint  REFERENCES projects (id),
    sales_invoice_id  bigint  REFERENCES sales_invoices (id) ON DELETE SET NULL,
    started_at        timestamptz NOT NULL,
    ended_at          timestamptz,
    description       text    NOT NULL,
    paused_duration   integer NOT NULL DEFAULT 0,
    billable          boolean NOT NULL DEFAULT true,
    created_at        timestamptz NOT NULL,
    updated_at        timestamptz NOT NULL
);
CREATE INDEX time_entries_admin_idx ON time_entries (administration_id, started_at DESC);
CREATE INDEX time_entries_invoice_idx ON time_entries (sales_invoice_id) WHERE sales_invoice_id IS NOT NULL;
CREATE INDEX time_entries_running_idx ON time_entries (administration_id, user_id) WHERE ended_at IS NULL;

-- task_list_templates are checklists task lists are made from, such as
-- closing a month; period and deadline_after are ISO 8601 durations.
CREATE TABLE task_list_templates (
    id                bigint PRIMARY KEY,
    administration_id bigint NOT NULL REFERENCES administrations (id) ON DELETE CASCADE,
    name              text   NOT NULL,
    description       text   NOT NULL DEFAULT '',
    period            text   NOT NULL DEFAULT '',
    deadline_after    text   NOT NULL DEFAULT '',
    created_at        timestamptz NOT NULL,
    updated_at        timestamptz NOT NULL
);
CREATE INDEX task_list_templates_admin_idx ON task_list_templates (administration_id);

CREATE TABLE task_lists (
    id                bigint PRIMARY KEY,
    administration_id bigint NOT NULL REFERENCES administrations (id) ON DELETE CASCADE,
    template_id       bigint REFERENCES task_list_templates (id) ON DELETE SET NULL,
    name              text   NOT NULL,
    description       text   NOT NULL DEFAULT '',
    deadline_on       date,
    period_begin      date,
    period_end        date,
    created_at        timestamptz NOT NULL,
    updated_at        timestamptz NOT NULL
);
CREATE INDEX task_lists_admin_idx ON task_lists (administration_id, created_at DESC);

-- A group belongs to a task list or a template. The first one, without a
-- name, holds the tasks outside any group and cannot be changed.
CREATE TABLE task_list_groups (
    id                bigint PRIMARY KEY,
    administration_id bigint  NOT NULL REFERENCES administrations (id) ON DELETE CASCADE,
    task_list_id      bigint  REFERENCES task_lists (id) ON DELETE CASCADE,
    template_id       bigint  REFERENCES task_list_templates (id) ON DELETE CASCADE,
    name              text,
    is_first          boolean NOT NULL DEFAULT false,
    row_order         integer NOT NULL,
    report_type       text    NOT NULL DEFAULT '',
    created_at        timestamptz NOT NULL,
    updated_at        timestamptz NOT NULL,
    CHECK ((task_list_id IS NULL) <> (template_id IS NULL))
);
CREATE INDEX task_list_groups_list_idx ON task_list_groups (task_list_id);
CREATE INDEX task_list_groups_template_idx ON task_list_groups (template_id);

-- Tasks generated from a report or a category cannot be deleted.
CREATE TABLE task_list_tasks (
    id                bigint PRIMARY KEY,
    administration_id bigint  NOT NULL REFERENCES administrations (id) ON DELETE CASCADE,
    group_id          bigint  NOT NULL REFERENCES task_list_groups (id) ON DELETE CASCADE,
    parent_task_id    bigint  REFERENCES task_list_tasks (id) ON DELETE CASCADE,
    name              text    NOT NULL,
    description       text    NOT NULL DEFAULT '',
    row_order         integer NOT NULL,
    completed_at      timestamptz,
    report_type       text    NOT NULL DEFAULT '',
    assignee_id       bigint  REFERENCES users (id) ON DELETE SET NULL,
    related_entity_id bigint,
    generated         boolean NOT NULL DEFAULT false,
    created_at        timestamptz NOT NULL,
    updated_at        timestamptz NOT NULL
);
CREATE INDEX task_list_tasks_group_idx ON task_list_tasks (group_id);
CREATE INDEX task_list_tasks_parent_idx ON task_list_tasks (parent_task_id);

-- +goose Down
DELETE FROM notes WHERE entity_type IN ('TimeEntry', 'TaskLists::Task');
DELETE FROM resources WHERE type IN ('time_entries', 'task_lists', 'task_list_templates', 'task_list_groups', 'task_list_tasks');
DROP TABLE task_list_tasks;
DROP TABLE task_list_groups;
DROP TABLE task_lists;
DROP TABLE task_list_templates;
DROP TABLE time_entries;
