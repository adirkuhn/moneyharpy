import { useEffect, useRef } from 'react'
import { useTranslation } from 'react-i18next'
import { Link, useNavigate, useSearchParams } from 'react-router-dom'
import { errorMessages } from '../api/client'
import { Problems } from '../auth/AuthLayout'
import { Loading } from '../layout/Loading'
import { Page } from '../layout/Page'
import { useCompleteBankConnection } from './queries'

/**
 * Where the bank sends the user back to after they authorised a
 * connection: it completes the connection and opens the financial
 * accounts, where the new transactions are.
 */
export function BankConnectionCallback() {
  const { t } = useTranslation()
  const [search] = useSearchParams()
  const navigate = useNavigate()
  const complete = useCompleteBankConnection()
  const sent = useRef(false)
  const state = search.get('state') ?? ''
  const code = search.get('code') ?? ''
  const refused = search.get('error_description') || search.get('error')

  useEffect(() => {
    if (sent.current || refused || !state || !code) {
      return
    }
    sent.current = true
    complete.mutate({ state, code }, { onSuccess: (c) => navigate(`/${c.administration_id}/financial_accounts`, { replace: true, state: { connected: c.bank } }) })
  }, [complete, code, navigate, refused, state])

  const problems = refused ? [t('bank.callback.refused', { reason: refused })] : !state || !code ? [t('bank.callback.incomplete')] : errorMessages(complete.error, {})
  return (
    <Page title={t('bank.callback.title')}>
      {problems.length === 0 ? (
        <>
          <p role="status">{t('bank.callback.completing')}</p>
          <Loading />
        </>
      ) : (
        <>
          <Problems messages={problems} />
          <p><Link to="/">{t('bank.callback.back')}</Link></p>
        </>
      )}
    </Page>
  )
}
