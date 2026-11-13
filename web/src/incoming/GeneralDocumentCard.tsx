import { type ReactNode, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useLocation } from 'react-router-dom'
import { useAdministration } from '../administrations/queries'
import { Problems } from '../auth/AuthLayout'
import { contactName } from '../contacts/queries'
import { DateInput } from '../forms/DatePicker'
import { formErrors, TextInput } from '../forms/Form'
import { ContactSelect } from '../invoices/ContactSelect'
import { DocumentFacts, DocumentForm, FactRows, type Saving } from './DocumentCard'
import { type GeneralDocument, useSaveGeneralDocument } from './queries'

type Props = {
  administrationId: string
  doc: GeneralDocument
  typeRow: ReactNode
  editing: boolean
  onEdit: () => void
  onSaved: (stillNew: boolean, next: boolean) => void
  left: number
  cancelTo: string
}

/** A general document: kept for reference, with a contact, a reference, a date and when it is due. */
export function GeneralDocumentCard(props: Props) {
  const { t } = useTranslation()
  const { doc } = props
  if (!props.editing) {
    return (
      <DocumentFacts label={t('incoming.types.general_document')} onEdit={props.onEdit}>
        <div className="incoming-card__rows">
          {props.typeRow}
          <FactRows
            rows={[
              [t('invoices.contact'), doc.contact ? contactName(doc.contact) : ''],
              [t('incoming.view.reference'), doc.reference],
              [t('incoming.view.date'), doc.date],
              [t('incoming.view.dueDate'), doc.due_date],
            ]}
          />
        </div>
      </DocumentFacts>
    )
  }
  return <GeneralForm key={doc.version} {...props} />
}

function GeneralForm({ administrationId, doc, typeRow, onSaved, left, cancelTo }: Props) {
  const { t } = useTranslation()
  const { pathname } = useLocation()
  const administration = useAdministration(administrationId)
  const save = useSaveGeneralDocument(administrationId, doc.id ?? '')
  const [contact, setContact] = useState({ id: doc.contact_id ?? '', name: doc.contact ? contactName(doc.contact) : '' })
  const [reference, setReference] = useState(doc.reference ?? '')
  const [date, setDate] = useState(doc.date ?? '')
  const [dueDate, setDueDate] = useState(doc.due_date ?? '')
  const labels = { contact_id: t('invoices.contact'), reference: t('incoming.view.reference'), date: t('incoming.view.date'), due_date: t('incoming.view.dueDate') }
  const errors = formErrors(save.error, labels)
  const saving: Saving = {
    pending: save.isPending,
    save: (next) =>
      save.mutate(
        { reference: reference.trim(), date, due_date: dueDate, ...(contact.id ? { contact_id: contact.id } : { removeContact: true }) },
        { onSuccess: (saved) => onSaved(saved.state === 'new', next) },
      ),
    left,
    cancelTo,
  }
  return (
    <DocumentForm label={t('incoming.types.general_document')} saving={saving}>
      <Problems messages={errors.general} />
      <div className="incoming-card__rows">
        {typeRow}
        <ContactSelect
          administrationId={administrationId}
          value={contact.name}
          errors={errors.field('contact_id')}
          homeCountry={administration.data?.country ?? 'NL'}
          returnTo={pathname}
          onChange={(c) => setContact({ id: String(c.id ?? ''), name: contactName(c) })}
        />
        <TextInput label={labels.reference} maxLength={255} value={reference} errors={errors.field('reference')} onChange={(e) => setReference(e.target.value)} />
        <DateInput label={labels.date} value={date} errors={errors.field('date')} onChange={setDate} />
        <DateInput label={labels.due_date} value={dueDate} errors={errors.field('due_date')} onChange={setDueDate} />
      </div>
    </DocumentForm>
  )
}
