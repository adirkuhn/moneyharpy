import { useTranslation } from 'react-i18next'
import { Link } from 'react-router-dom'
import { ChevronIcon } from '../layout/icons'
import { Block } from '../settings/Block'
import { useWorkflows } from './queries'

/**
 * Moneybird's online payment methods: the invoice workflows with the methods
 * they offer. MoneyHarpy has no payment provider yet, so nothing is offered.
 */
export function PaymentMethodsBlock({ administrationId }: { administrationId: string }) {
  const { t } = useTranslation()
  const workflows = useWorkflows(administrationId)
  const invoiceFlows = workflows.data?.filter((w) => w.type === 'InvoiceWorkflow') ?? []
  return (
    <Block
      id="payment-methods"
      title={t('invoicing.parts.payments')}
      subtitle={t('invoicing.paymentsSubtitle')}
      action={<button type="button" className="btn btn--accent" disabled title={t('list.notAvailable')}>{t('invoicing.offerPayments')}</button>}
    >
      <p className="empty">{t('invoicing.paymentsUnavailable')}</p>
      <table className="data-table data-table--rows" aria-label={t('invoicing.workflowsTable')}>
        <thead>
          <tr>
            <th scope="col">{t('invoicing.workflow')}</th>
            <th scope="col">{t('invoicing.paymentMethods')}</th>
          </tr>
        </thead>
        <tbody>
          {invoiceFlows.map((w) => (
            <tr key={w.id}>
              <td>{w.name}</td>
              <td>-</td>
            </tr>
          ))}
        </tbody>
      </table>
      <div className="card card--with-footer">
        <div className="switch-row is-disabled">
          <div>
            <p className="switch-row__label">{t('invoicing.costs')}</p>
            <p className="hint">{t('invoicing.costsHelp')}</p>
          </div>
          <ChevronIcon className="ledger__chevron" />
        </div>
        <p className="card__footer">
          {t('invoicing.externalBefore')} <Link to={`/${administrationId}/settings/connections`}>{t('invoicing.externalLink')}</Link> {t('invoicing.externalAfter')}
        </p>
      </div>
    </Block>
  )
}
