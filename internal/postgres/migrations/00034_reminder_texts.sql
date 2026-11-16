-- +goose Up
-- Workflows made by MoneyHarpy's first default kept one-line reminder texts
-- without a greeting, closing or {reminders_table}. Those still unchanged
-- become today's defaults, after Moneybird's: greeting, text, the table of
-- unpaid invoices, closing and the sender. Texts anyone edited stay.
-- +goose StatementBegin
UPDATE workflow_steps SET reminder_text = $new$Dear {recipient.name},

According to our records, the invoices below have not been paid yet. We would like to point out that the payment term of these invoices has expired and kindly but urgently request you to pay as soon as possible.

{reminders_table}

If this reminder and your payment have crossed, please disregard this reminder.

Kind regards,

{company.company_name}$new$ WHERE reminder_text = $old$We have not received payment for this invoice yet. Please pay the outstanding amount as soon as possible.$old$;
-- +goose StatementEnd
-- +goose StatementBegin
UPDATE workflow_steps SET reminder_text = $new$Dear {recipient.name},

Despite an earlier reminder, our records show that the invoices below have not been paid yet. We would like to point out that the payment term of these invoices has expired and kindly but urgently request you to pay as soon as possible.

{reminders_table}

If this reminder and your payment have crossed, please disregard this reminder. If payment is not received, we will have to hand the claim over to a debt collection agency and charge the statutory collection costs.

Kind regards,

{company.company_name}$new$ WHERE reminder_text = $old$This is a second reminder: we still have not received payment for this invoice. Please pay the outstanding amount within 7 days.$old$;
-- +goose StatementEnd
-- +goose StatementBegin
UPDATE workflow_steps SET reminder_text = $new$Geachte {recipient.name},

Uit onze administratie blijkt dat u de onderstaande facturen nog niet voldaan heeft. We willen u erop wijzen dat de betaaltermijn van deze facturen verstreken is en verzoeken u vriendelijk doch dringend om de betaling zo spoedig mogelijk te voldoen.

{reminders_table}

Mochten de betalingen en deze herinnering elkaar kruisen, dan kunt u deze herinnering als niet verzonden beschouwen.

Met vriendelijke groet,

{company.company_name}$new$ WHERE reminder_text = $old$We hebben de betaling voor deze factuur nog niet ontvangen. Wil je het openstaande bedrag zo snel mogelijk betalen?$old$;
-- +goose StatementEnd
-- +goose StatementBegin
UPDATE workflow_steps SET reminder_text = $new$Geachte {recipient.name},

Ondanks een eerdere herinnering blijkt uit onze administratie dat u de onderstaande facturen nog niet voldaan heeft. We willen u er op wijzen dat de betaaltermijn van deze facturen verstreken is en verzoeken u vriendelijk doch dringend om de betaling zo spoedig mogelijk te voldoen.

{reminders_table}

Mochten de betalingen en deze herinnering elkaar kruisen, dan kunt u deze herinnering als niet verzonden beschouwen. Wanneer betaling uitblijft, zullen wij de vordering uit handen moeten geven aan een incassobureau en de wettelijke incassokosten in rekening brengen.

Met vriendelijke groet,

{company.company_name}$new$ WHERE reminder_text = $old$Dit is een tweede herinnering: we hebben de betaling voor deze factuur nog steeds niet ontvangen. Wil je het openstaande bedrag binnen 7 dagen betalen?$old$;
-- +goose StatementEnd

-- +goose Down
-- The old one-line texts are not restored.
SELECT 1;
