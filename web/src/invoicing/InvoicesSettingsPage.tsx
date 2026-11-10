import { useEffect } from 'react'
import { useTranslation } from 'react-i18next'
import { useLocation, useParams } from 'react-router-dom'
import { useAdministration } from '../administrations/queries'
import { Loading } from '../layout/Loading'
import { Page } from '../layout/Page'
import { Block } from '../settings/Block'
import { CustomFieldsBlock } from './CustomFieldsBlock'
import { ForwardingBlock } from './ForwardingBlock'
import { NumberingBlock } from './NumberingBlock'
import { useInvoiceSettings, useUpdateInvoiceSettings } from './queries'
import { StylesBlock } from './StylesBlock'
import { SwitchRow } from './Switch'
import { WorkflowsBlock } from './WorkflowsBlock'

/** Settings > Invoices and estimates, block by block like Moneybird. */
export function InvoicesSettingsPage() {
  const { t } = useTranslation()
  const { administrationId = '' } = useParams()
  const administration = useAdministration(administrationId)
  const settings = useInvoiceSettings(administrationId)
  const update = useUpdateInvoiceSettings(administrationId)
  const { hash } = useLocation()
  const ready = Boolean(settings.data && administration.data)

  useEffect(() => {
    if (hash && ready) {
      document.getElementById(hash.slice(1))?.scrollIntoView?.({ block: 'start' })
    }
  }, [hash, ready])

  if (!settings.data || !administration.data) {
    return <Loading />
  }
  const s = settings.data
  return (
    <Page title={t('invoicing.title')} subtitle={t('invoicing.subtitle')}>
      <StylesBlock administrationId={administrationId} />
      <WorkflowsBlock administrationId={administrationId} />
      <NumberingBlock key={`${s.invoice_id_template}${s.invoice_sequence_id}${s.estimate_id_template}${s.estimate_sequence_id}`} administrationId={administrationId} settings={s} language={administration.data.language} />
      <Block id="peppol-ubl" title={t('invoicing.parts.peppol')} subtitle={t('invoicing.peppolSubtitle')}>
        <div className="card">
          <p className="card__title">{t('invoicing.peppol')}</p>
          <p className="empty">{t('invoicing.peppolUnavailable')}</p>
        </div>
        <div className="card">
          <SwitchRow label={t('invoicing.ubl')} help={t('invoicing.ublHelp')} checked={s.default_email_ubl} disabled={update.isPending} onChange={(on) => update.mutate({ default_email_ubl: on })} />
        </div>
      </Block>
      <CustomFieldsBlock administrationId={administrationId} />
      <ForwardingBlock key={s.invoice_bcc_email_address + s.estimate_bcc_email_address + s.order_bcc_email_address + String(s.invoice_reminders_bcc_enabled)} administrationId={administrationId} settings={s} />
      <Block id="payment-methods" title={t('invoicing.parts.payments')} subtitle={t('invoicing.paymentsSubtitle')}>
        <p className="empty">{t('invoicing.paymentsUnavailable')}</p>
      </Block>
      <Block id="payments-without-proof" title={t('invoicing.parts.withoutProof')} subtitle={t('invoicing.withoutProofSubtitle')}>
        <div className="card">
          <SwitchRow label={t('invoicing.allow')} help={t('invoicing.withoutProofHelp')} checked={s.payments_without_proof} disabled={update.isPending} onChange={(on) => update.mutate({ payments_without_proof: on })} />
        </div>
      </Block>
    </Page>
  )
}
