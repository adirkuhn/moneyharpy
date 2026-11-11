import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { type ListFilter, termOf, withTerm } from '../invoices/queries'
import { useWorkflows } from '../invoicing/queries'
import { AdvancedFilter, ContactFilter, RadioFilter, SelectFilter } from './FilterPanel'
import { parsePeriod, periodValue } from './period'
import { PeriodPicker } from './PeriodPicker'

type Props = {
  administrationId: string
  filter: ListFilter
  /** The states to choose from, the first being all of them. */
  states: { value: string; label: string }[]
  /** Whether the list can be filtered on workflow. */
  workflows?: boolean
  onApply: (filter: ListFilter) => void
}

/** The advanced filter of the invoice lists: a period, a status, a contact and, for sales invoices, a workflow. */
export function DocumentFilter({ administrationId, filter, states, workflows = false, onApply }: Props) {
  const { t } = useTranslation()
  const today = useMemo(() => new Date(), [])
  const [range, setRange] = useState(() => parsePeriod(filter.period, today))
  const [periodTouched, setPeriodTouched] = useState(false)
  const [state, setState] = useState(filter.state)
  const [contactId, setContactId] = useState(termOf(filter.rest, 'contact_id'))
  const [workflowId, setWorkflowId] = useState(termOf(filter.rest, 'workflow_id'))

  const apply = () =>
    onApply({
      state,
      // A period the picker cannot show stays as it is until another is picked.
      period: periodTouched ? periodValue(range, today) : filter.period,
      rest: withTerm(withTerm(filter.rest, 'contact_id', contactId), 'workflow_id', workflows ? workflowId : termOf(filter.rest, 'workflow_id')),
    })

  return (
    <AdvancedFilter onApply={apply}>
      <PeriodPicker
        value={range}
        today={today}
        onChange={(r) => {
          setRange(r)
          setPeriodTouched(true)
        }}
      />
      <RadioFilter legend={t('list.status')} value={state} onChange={setState} options={states} />
      <ContactFilter administrationId={administrationId} contactId={contactId} onChange={setContactId} />
      {workflows && <WorkflowFilter administrationId={administrationId} value={workflowId} onChange={setWorkflowId} />}
    </AdvancedFilter>
  )
}

/** The invoice workflows to filter on, or all of them. */
export function WorkflowFilter({ administrationId, value, onChange }: { administrationId: string; value: string; onChange: (id: string) => void }) {
  const { t } = useTranslation()
  const list = useWorkflows(administrationId)
  const options = (list.data ?? []).filter((w) => w.type === 'InvoiceWorkflow' || w.id === value)
  return (
    <SelectFilter
      label={t('list.workflow')}
      value={value}
      onChange={onChange}
      options={[{ value: '', label: t('list.allWorkflows') }, ...options.map((w) => ({ value: w.id ?? '', label: w.name ?? '' }))]}
    />
  )
}
