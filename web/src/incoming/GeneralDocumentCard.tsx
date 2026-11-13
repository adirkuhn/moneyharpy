import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { contactName } from '../contacts/queries'
import { NextFooter } from './DocumentCard'
import { type FieldSaving, fieldErrors, InlineContact, InlineDate, InlineText } from './InlineFields'
import { type GeneralDocument, useSaveGeneralDocument } from './queries'

type Props = {
  administrationId: string
  doc: GeneralDocument
  typeRow: ReactNode
  /** How many other documents are still to process. */
  left: number
  /** Opens the next document to process, for a document that is still new. */
  onNext?: () => void
  /** Shown above the fields, such as a warning of a possible duplicate. */
  notice?: ReactNode
}

type Field = 'contact_id' | 'reference' | 'date' | 'due_date'

/** Saves one field of a general document with a request of its own. */
function useGeneralField(administrationId: string, id: string, name: Field, label: string): FieldSaving {
  const save = useSaveGeneralDocument(administrationId, id)
  return { save: (value) => save.mutate({ [name]: value }), pending: save.isPending, errors: fieldErrors(save.error, name, label) }
}

/**
 * A general document: kept for reference, with a contact, a reference, a date
 * and when it is due, each saved on its own as on Moneybird's document page.
 */
export function GeneralDocumentCard({ administrationId, doc, typeRow, left, onNext, notice }: Props) {
  const { t } = useTranslation()
  const id = doc.id ?? ''
  const contact = useGeneralField(administrationId, id, 'contact_id', t('invoices.contact'))
  const reference = useGeneralField(administrationId, id, 'reference', t('incoming.view.reference'))
  const date = useGeneralField(administrationId, id, 'date', t('incoming.view.date'))
  const due = useGeneralField(administrationId, id, 'due_date', t('incoming.view.dueDate'))
  return (
    <section className="incoming-card" aria-label={t('incoming.types.general_document')}>
      {notice}
      <div className="incoming-card__rows">
        {typeRow}
        <InlineContact administrationId={administrationId} name={doc.contact ? contactName(doc.contact) : ''} saving={contact} />
        <InlineText label={t('incoming.view.reference')} saved={doc.reference ?? ''} saving={reference} />
        <InlineDate label={t('incoming.view.date')} saved={doc.date ?? ''} saving={date} />
        <InlineDate label={t('incoming.view.dueDate')} saved={doc.due_date ?? ''} saving={due} />
      </div>
      {doc.state === 'new' && <NextFooter left={left} onNext={onNext} />}
    </section>
  )
}
