import { useQueryClient } from '@tanstack/react-query'
import { type FormEvent, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useNavigate, useParams } from 'react-router-dom'
import { ApiError } from '../api/client'
import { Problems } from '../auth/AuthLayout'
import { formErrors } from '../forms/Form'
import { NotesAndHistory } from '../invoices/InvoiceHistory'
import { JournalEntryForm } from '../journal/JournalDocumentPage'
import { Loading } from '../layout/Loading'
import { DangerDialog } from '../layout/Modal'
import { NotFoundPage } from '../layout/NotFoundPage'
import { Page } from '../layout/Page'
import { PageIcon } from '../layout/pageIcons'
import { type MenuItem, MoreMenu } from '../list/ListActions'
import { AttachmentViewer } from './AttachmentViewer'
import { documentPath, useDocumentSummary, useToProcess } from './documents'
import { GeneralDocumentCard } from './GeneralDocumentCard'
import { incomingHistory } from './history'
import { PurchaseDocument } from './PurchaseDocument'
import {
  choiceOf, incomingKey, type TypedDocument, useAddDocumentNote, useDeleteDocument, useDeleteDocumentNote, useSetDocumentType, useTypedDocument,
} from './queries'
import { TypePicker } from './TypePicker'
import '../invoices/view.css'
import './document.css'

/**
 * An incoming document of any type (/documents/:id), as Moneybird's one page
 * per document: the attachment at the left, the document's form at the
 * right, with its notes and history under it.
 */
export function DocumentPage() {
  const { administrationId = '', documentId = '' } = useParams()
  const summary = useDocumentSummary(administrationId, documentId)
  const typed = useTypedDocument(administrationId, summary.data?.type, documentId)
  const error = summary.error ?? typed.error
  if (error instanceof ApiError && error.status < 500) {
    return <NotFoundPage />
  }
  if (!summary.data || !typed.data) {
    return <Loading />
  }
  return <DocumentView key={`${typed.data.type}-${documentId}`} administrationId={administrationId} typed={typed.data} />
}

/** What the page is called: the type and the reference, as Moneybird's "Receipt 2026-0042"; an upload is its file. */
function useTitle({ type, doc }: TypedDocument) {
  const { t } = useTranslation()
  const choice = choiceOf[type]
  if (!choice) {
    return doc.reference || doc.attachments?.[0]?.filename || t('incoming.view.document')
  }
  const name = t(`incoming.types.${choice}`)
  return doc.reference ? `${name} ${doc.reference}` : name
}

function DocumentView({ administrationId, typed }: { administrationId: string; typed: TypedDocument }) {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const client = useQueryClient()
  const { type, doc } = typed
  const id = doc.id ?? ''
  const list = `/${administrationId}/documents`
  const title = useTitle(typed)
  const isNew = 'state' in doc && doc.state === 'new'
  const purchase = typed.type === 'PurchaseInvoice' || typed.type === 'Receipt'
  // A paid document, or one with payments, keeps what it booked, as the server refuses to change it.
  const editable = purchase ? typed.doc.state !== 'paid' && (typed.doc.payments ?? []).length === 0 : type === 'GeneralDocument'
  // A journal entry or a booked document cannot change type.
  const canChangeType = type === 'TypelessDocument' || type === 'GeneralDocument' || (purchase && isNew)
  const [editing, setEditing] = useState(isNew)
  const [deleting, setDeleting] = useState(false)
  const setType = useSetDocumentType(administrationId, id)
  const remove = useDeleteDocument(administrationId, type, id)
  const addNote = useAddDocumentNote(administrationId, type, id)
  const removeNote = useDeleteDocumentNote(administrationId, type, id)
  const toProcess = useToProcess(administrationId, id)

  const onSaved = (stillNew: boolean, next: boolean) => {
    if (next) {
      navigate(toProcess.next ? documentPath(administrationId, toProcess.next) : list)
    } else if (!stillNew) {
      setEditing(false)
    }
  }
  const edit = editable && !editing ? () => setEditing(true) : undefined
  const typeRow = (
    <TypePicker
      value={choiceOf[type]}
      disabled={!canChangeType || setType.isPending}
      errors={setType.error ? formErrors(setType.error, {}).general : undefined}
      onChange={(choice) => setType.mutate(choice)}
    />
  )
  const shared = { administrationId, typeRow, editing, onSaved, left: toProcess.left, cancelTo: list }

  const card = (() => {
    switch (typed.type) {
      case 'PurchaseInvoice':
      case 'Receipt':
        return <PurchaseDocument {...shared} kind={typed.type === 'Receipt' ? 'receipts' : 'purchase_invoices'} doc={typed.doc} onEdit={edit} />
      case 'GeneralDocument':
        return <GeneralDocumentCard {...shared} doc={typed.doc} onEdit={() => setEditing(true)} />
      case 'GeneralJournalDocument':
        return (
          <section className="incoming-card" aria-label={t('incoming.types.general_journal_document')}>
            <div className="incoming-card__rows">{typeRow}</div>
            <JournalEntryForm
              embedded
              administrationId={administrationId}
              doc={typed.doc}
              cancelTo={list}
              onSaved={() => void client.invalidateQueries({ queryKey: incomingKey(administrationId, id) })}
            />
          </section>
        )
      default:
        return (
          <form className="incoming-card" aria-label={t('incoming.view.document')} onSubmit={(e: FormEvent) => e.preventDefault()}>
            <div className="incoming-card__rows">{typeRow}</div>
            <p className="incoming-card__hint">{t('incoming.view.chooseTypeText')}</p>
          </form>
        )
    }
  })()

  const more: MenuItem[] = [
    ...(edit ? [{ label: t('incoming.view.edit'), onSelect: edit }] : []),
    { label: t('incoming.new.back'), to: list },
  ]

  return (
    <Page
      wide
      title={title}
      actions={
        <>
          <button type="button" className="btn btn--secondary btn--icon" aria-label={t('incoming.view.deleteDocument')} title={t('incoming.view.deleteDocument')} onClick={() => setDeleting(true)}>
            <PageIcon kind="trash" />
          </button>
          <MoreMenu items={more} />
        </>
      }
    >
      <Problems messages={formErrors(remove.error, {}).general} />
      <div className="incoming-document">
        <div className="incoming-document__left">
          <AttachmentViewer administrationId={administrationId} type={type} documentId={id} attachments={doc.attachments ?? []} />
        </div>
        <div className="incoming-document__right">
          {card}
          {type !== 'TypelessDocument' && 'notes' in doc && (
            <NotesAndHistory administrationId={administrationId} items={incomingHistory(doc)} currency={'currency' in doc ? (doc.currency ?? 'EUR') : 'EUR'} add={addNote} remove={removeNote} />
          )}
        </div>
      </div>
      {deleting && (
        <DangerDialog
          question={t('incoming.view.confirmDelete')}
          busy={remove.isPending}
          onCancel={() => setDeleting(false)}
          onConfirm={() => remove.mutate(undefined, { onSuccess: () => navigate(list), onError: () => setDeleting(false) })}
        />
      )}
    </Page>
  )
}
