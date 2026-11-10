import { type FormEvent, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Problems } from '../auth/AuthLayout'
import { formErrors, FormNotice, TextInput } from '../forms/Form'
import { useConfirmTwoFactor, useDisableTwoFactor, useNewRecoveryCodes, useStartTwoFactor, useTwoFactor } from '../management/queries'

/** Two-step verification of the user's own account: set up, recovery codes, turn off. */
export function TwoFactorBlock() {
  const { t } = useTranslation()
  const status = useTwoFactor()
  const start = useStartTwoFactor()
  const confirm = useConfirmTwoFactor()
  const disable = useDisableTwoFactor()
  const renew = useNewRecoveryCodes()
  const [code, setCode] = useState('')
  const [codes, setCodes] = useState<string[]>([])
  const error = confirm.error ?? disable.error ?? renew.error
  const errors = formErrors(error, { code: t('twoFactor.code') })
  const done = (next: { recovery_codes: string[] }) => {
    setCodes(next.recovery_codes)
    setCode('')
  }
  const s = status.data

  const codeField = <TextInput label={t('twoFactor.code')} width="medium" inputMode="text" autoComplete="one-time-code" maxLength={32} value={code} errors={errors.field('code')} onChange={(e) => setCode(e.target.value)} />

  return (
    <section id="two-step" className="settings-block" aria-labelledby="two-step-title">
      <header className="settings-block__header">
        <div>
          <h2 id="two-step-title">{t('twoFactor.title')}</h2>
          <p className="hint">{t('twoFactor.subtitle')}</p>
        </div>
      </header>
      <div className="card card-stack">
        {codes.length > 0 && (
          <div className="secret" role="status">
            <p>{t('twoFactor.saveCodes')}</p>
            <ul className="recovery-codes">{codes.map((c) => <li key={c}><code>{c}</code></li>)}</ul>
          </div>
        )}
        <Problems messages={errors.general} />
        {s && !s.enabled && !start.data && (
          <>
            {s.required && <FormNotice>{t('twoFactor.requiredNotice')}</FormNotice>}
            <p>{t('twoFactor.off')}</p>
            <div><button type="button" className="btn btn--accent" disabled={start.isPending} onClick={() => start.mutate()}>{t('twoFactor.setUp')}</button></div>
          </>
        )}
        {s && !s.enabled && start.data && (
          <form className="card-stack" onSubmit={(e: FormEvent) => { e.preventDefault(); confirm.mutate(code, { onSuccess: done }) }} noValidate>
            <p>{t('twoFactor.scan')}</p>
            <img className="qr" alt={t('twoFactor.qrAlt')} src={`data:image/svg+xml;utf8,${encodeURIComponent(start.data.qr_svg)}`} width={192} height={192} />
            <p className="hint">{t('twoFactor.orType')} <code className="secret-inline">{start.data.secret}</code></p>
            {codeField}
            <div><button className="btn btn--primary" disabled={confirm.isPending}>{t('twoFactor.turnOn')}</button></div>
          </form>
        )}
        {s?.enabled && (
          <form className="card-stack" onSubmit={(e: FormEvent) => e.preventDefault()} noValidate>
            <p>{t('twoFactor.on', { count: s.recovery_codes_left })}</p>
            {codeField}
            <div className="confirm-row__buttons">
              <button type="button" className="btn btn--secondary" disabled={renew.isPending || !code} onClick={() => renew.mutate(code, { onSuccess: done })}>{t('twoFactor.newCodes')}</button>
              <button type="button" className="btn btn--secondary" disabled={disable.isPending || !code || s.required} onClick={() => disable.mutate(code, { onSuccess: () => setCode('') })}>{t('twoFactor.turnOff')}</button>
            </div>
            {s.required && <p className="hint">{t('twoFactor.cannotTurnOff')}</p>}
          </form>
        )}
      </div>
    </section>
  )
}
