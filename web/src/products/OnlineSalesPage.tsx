import { ActionLink } from '../layout/ActionLink'
import { useId, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useParams, useSearchParams } from 'react-router-dom'
import { InfoIcon, ListIcon } from '../layout/icons'
import { Page } from '../layout/Page'
import { GettingStarted } from '../list/GettingStarted'
import { ActionMenu } from '../list/ListActions'
import './products.css'

/**
 * Online sales (/payment_links), after Moneybird's: what selling online is,
 * and Add for a payment link or a subscription template. MoneyHarpy takes no
 * online payments, so these lead to the page that says what is missing.
 */
export function OnlineSalesPage() {
  const { t } = useTranslation()
  const { administrationId = '' } = useParams()
  const [info, setInfo] = useState(true)
  const introId = useId()
  const base = `/${administrationId}`
  return (
    <Page
      wide
      title={t('onlineSales.title')}
      actions={
        <>
          <button
            type="button"
            className={`btn btn--secondary btn--icon${info ? ' is-active' : ''}`}
            aria-label={t('list.moreInfo')}
            title={t('list.moreInfo')}
            aria-expanded={info}
            aria-controls={introId}
            onClick={() => setInfo(!info)}
          >
            <InfoIcon />
          </button>
          <ActionMenu
            primary
            label={t('onlineSales.add')}
            icon={<ListIcon kind="plus" />}
            items={[
              { label: t('onlineSales.paymentLink'), to: `${base}/payment_links/new` },
              { label: t('onlineSales.subscriptionTemplate'), to: `${base}/subscription_templates/new` },
            ]}
          />
        </>
      }
    >
      {info && (
        <GettingStarted
          id={introId}
          title={t('onlineSales.intro.title')}
          text={t('onlineSales.intro.text')}
          steps={[
            { icon: 'plus', title: t('onlineSales.intro.add'), text: t('onlineSales.intro.addText') },
            { icon: 'card', title: t('onlineSales.intro.checkout'), text: t('onlineSales.intro.checkoutText') },
            { icon: 'check', title: t('onlineSales.intro.processed'), text: t('onlineSales.intro.processedText') },
          ]}
        />
      )}
    </Page>
  )
}

/**
 * What Moneybird shows before its checkout is set up (/payment_links/incomplete),
 * which in MoneyHarpy is always: online payment methods, and a workflow that
 * offers them. A product's payment link (/payment_links/:id/edit) leads back
 * to the product.
 */
export function PaymentLinkNotReadyPage() {
  const { t } = useTranslation()
  const { administrationId = '' } = useParams()
  const [search] = useSearchParams()
  const asked = search.get('return_to') ?? ''
  // Only a page of this administration, so the link cannot lead elsewhere.
  const product = asked.startsWith(`/${administrationId}/products/`) ? asked : ''
  const back = product
    ? { to: product, label: t('onlineSales.backToProduct') }
    : { to: `/${administrationId}/payment_links`, label: t('onlineSales.back') }
  return (
    <Page title={back.label} back={back} wide>
      <section className="online-not-ready" aria-labelledby="online-not-ready">
        <div className="online-not-ready__intro">
          <h2 id="online-not-ready">{t('onlineSales.notReady.title')}</h2>
          <p>{t('onlineSales.notReady.text')}</p>
        </div>
        <ol className="online-not-ready__steps" aria-label={t('onlineSales.notReady.needs')}>
          <li><ListIcon kind="card" /><span>{t('onlineSales.notReady.methods')}</span></li>
          <li><ListIcon kind="flow" /><span>{t('onlineSales.notReady.workflow')}</span></li>
        </ol>
        <div className="online-not-ready__actions">
          <ActionLink className="btn btn--secondary" to={back.to}>{back.label}</ActionLink>
        </div>
      </section>
    </Page>
  )
}
