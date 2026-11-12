-- +goose Up
-- Workflow steps take Moneybird's meaning. Moneybird counts steps from 0
-- (0 sends the invoice, 1 and on are the reminders) and every step expires
-- after its own due_interval ("Reminder expires after"); when a step
-- expires unpaid the next reminder is due: the first on the due date, every
-- later one the previous reminder's due_interval after it went out.
-- reminder_delay is always 0 on Moneybird.
--
-- Until now a reminder step held reminder_delay, the days after the step
-- before it that the reminder was due, and a due_interval of 0. The gap
-- between reminders n-1 and n moves to reminder n-1's due_interval, so
-- reminders after the first keep their spacing; the first reminder moves to
-- the due date, as on Moneybird, which has no gap there. The last
-- reminder, which nothing follows, expires after its own former gap.

-- Invoices remember the day their last reminder went out, which the next
-- reminder counts from: the day of the latest reminder event, or else the
-- day the old schedule planned it.
ALTER TABLE sales_invoices ADD COLUMN last_reminder_on date;

UPDATE sales_invoices i
SET last_reminder_on = coalesce(
    (SELECT max((e.created_at AT TIME ZONE a.time_zone)::date)
     FROM events e JOIN administrations a ON a.id = e.administration_id
     WHERE e.administration_id = i.administration_id AND e.entity_type = 'SalesInvoice' AND e.entity_id = i.id
       AND e.action IN ('sales_invoice_send_reminder_email', 'sales_invoice_send_reminder_manually')),
    i.due_date + (SELECT coalesce(sum(s.reminder_delay), 0)::integer
                  FROM workflow_steps s
                  WHERE s.workflow_id = i.workflow_id AND s.step_order BETWEEN 2 AND i.reminder_count + 1))
WHERE i.reminder_count > 0 AND i.due_date IS NOT NULL;

UPDATE workflow_steps s
SET due_interval = coalesce(
        (SELECT n.reminder_delay FROM workflow_steps n WHERE n.workflow_id = s.workflow_id AND n.step_order = s.step_order + 1),
        s.reminder_delay)
WHERE s.step_order > 1;

-- Step order counts from 0; flipping the sign first keeps it unique.
UPDATE workflow_steps SET reminder_delay = 0, step_order = -step_order;
UPDATE workflow_steps SET step_order = -step_order - 1;

-- +goose Down
UPDATE workflow_steps SET step_order = -(step_order + 1);
UPDATE workflow_steps SET step_order = -step_order;

UPDATE workflow_steps s
SET reminder_delay = coalesce(
        (SELECT p.due_interval FROM workflow_steps p WHERE p.workflow_id = s.workflow_id AND p.step_order = s.step_order - 1 AND p.step_order > 1),
        1)
WHERE s.step_order > 1;
UPDATE workflow_steps SET due_interval = 0 WHERE step_order > 1;

ALTER TABLE sales_invoices DROP COLUMN last_reminder_on;
