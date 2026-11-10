import { type FormEvent, useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link, useLocation, useNavigate, useParams } from 'react-router-dom'
import { useAdministration } from '../administrations/queries'
import { type Administration, errorMessages, type Language } from '../api/client'
import { Problems } from '../auth/AuthLayout'
import { useCurrentUser } from '../auth/queries'
import { FormSection, formErrors, SelectInput, TextInput } from '../forms/Form'
import { SwitchRow } from '../invoicing/Switch'
import { Loading } from '../layout/Loading'
import { Menu } from '../layout/Menu'
import { Page } from '../layout/Page'
import { useOnboardingOptions } from '../onboarding/queries'
import { SearchableSelect } from '../onboarding/SearchableSelect'
import { Block } from '../settings/Block'
import { useUpdateAdministration } from '../settings/queries'
import { can, useInvitations, useLeave, useRequireTwoFactor, useRevokeInvitation, userTypes, useUsers } from './queries'

function timeZones(current: string): string[] {
  const zones = typeof Intl.supportedValuesOf === 'function' ? Intl.supportedValuesOf('timeZone') : []
  return zones.includes(current) ? zones : [current, ...zones]
}

/** Settings > Management, block by block like Moneybird. */
export function ManagementSettingsPage() {
  const { t } = useTranslation()
  const { administrationId = '' } = useParams()
  const administration = useAdministration(administrationId)
  const { hash } = useLocation()
  const ready = Boolean(administration.data)

  useEffect(() => {
    if (hash && ready) {
      document.getElementById(hash.slice(1))?.scrollIntoView?.({ block: 'start' })
    }
  }, [hash, ready])

  if (!administration.data) {
    return <Loading />
  }
  const a = administration.data
  return (
    <Page title={t('settings.sections.administration')} subtitle={t('management.subtitle')}>
      <DetailsBlock key={a.id} administration={a} />
      {can(a, 'settings') && <UsersBlock administration={a} />}
      <SecurityBlock administration={a} />
      <PrivacyBlock administration={a} />
      <LeaveBlock administration={a} />
    </Page>
  )
}

function DetailsBlock({ administration: a }: { administration: Administration }) {
  const { t, i18n } = useTranslation()
  const options = useOnboardingOptions()
  const update = useUpdateAdministration(a.id)
  const [form, setForm] = useState({ name: a.name, country: a.country, language: a.language, currency: a.currency, time_zone: a.time_zone })
  const editable = can(a, 'settings')

  const countryOptions = useMemo(() => {
    const names = new Intl.DisplayNames([i18n.language], { type: 'region', fallback: 'code' })
    return (options.data?.countries ?? [form.country]).map((value) => ({ value, label: names.of(value) ?? value })).sort((x, y) => x.label.localeCompare(y.label, i18n.language))
  }, [options.data, form.country, i18n.language])
  const currencyOptions = useMemo(() => {
    const names = new Intl.DisplayNames([i18n.language], { type: 'currency', fallback: 'code' })
    return (options.data?.currencies ?? [form.currency]).map((value) => ({ value, label: `${value} - ${names.of(value) ?? value}` }))
  }, [options.data, form.currency, i18n.language])
  const zoneOptions = useMemo(() => timeZones(form.time_zone).map((value) => ({ value, label: value })), [form.time_zone])

  const submit = (e: FormEvent) => {
    e.preventDefault()
    update.mutate(form)
  }
  const labels = { name: t('management.administrationName'), country: t('administrations.country'), language: t('common.language'), currency: t('onboarding.currency'), time_zone: t('administrations.timeZone') }
  const errors = formErrors(update.error, labels)

  return (
    <Block id="administration-details" title={t('management.parts.details')} subtitle={t('management.detailsSubtitle')}>
      <form className="form-sheet form-sheet--block" onSubmit={submit} noValidate aria-label={t('management.parts.details')}>
        <fieldset className="card__fieldset" disabled={!editable}>
          <Problems messages={errors.general} />
          <FormSection>
            <TextInput label={labels.name} required maxLength={255} autoComplete="off" value={form.name} errors={errors.field('name')} onChange={(e) => setForm({ ...form, name: e.target.value })} />
            <SearchableSelect
              label={labels.country}
              searchLabel={t('onboarding.findCountry')}
              emptyLabel={t('onboarding.noOption')}
              value={form.country}
              options={countryOptions}
              width="medium"
              errors={errors.field('country')}
              help={form.country !== a.country ? t('management.countryHelp') : undefined}
              onChange={(country) => setForm({ ...form, country })}
            />
            <SelectInput label={labels.language} width="medium" value={form.language} errors={errors.field('language')} onChange={(e) => setForm({ ...form, language: e.target.value as Language })}>
              <option value="nl">Nederlands</option>
              <option value="nl-be">Nederlands (België)</option>
              <option value="en">English</option>
            </SelectInput>
          </FormSection>
          <FormSection>
            <SearchableSelect label={labels.currency} searchLabel={t('onboarding.findCurrency')} emptyLabel={t('onboarding.noOption')} value={form.currency} options={currencyOptions} width="medium" errors={errors.field('currency')} onChange={(currency) => setForm({ ...form, currency })} />
            <SearchableSelect label={labels.time_zone} searchLabel={t('settings.findTimeZone')} emptyLabel={t('onboarding.noOption')} value={form.time_zone} options={zoneOptions} width="medium" errors={errors.field('time_zone')} onChange={(time_zone) => setForm({ ...form, time_zone })} />
          </FormSection>
          {editable && (
            <div className="form-sheet__actions">
              <button className="btn btn--primary" disabled={update.isPending}>{t('settings.save')}</button>
              {update.isSuccess && <p role="status" className="saved">{t('settings.saved')}</p>}
            </div>
          )}
        </fieldset>
      </form>
    </Block>
  )
}

function UsersBlock({ administration: a }: { administration: Administration }) {
  const { t, i18n } = useTranslation()
  const owner = can(a, 'ownership')
  const users = useUsers(a.id)
  const invitations = useInvitations(a.id, owner)
  const revoke = useRevokeInvitation(a.id)
  const date = (s: string) => new Date(s).toLocaleDateString(i18n.language, { dateStyle: 'medium' })
  return (
    <Block
      id="users"
      title={t('management.parts.users')}
      subtitle={t('management.usersSubtitle')}
      action={
        owner && (
          <Menu label={t('management.invite')} buttonClassName="btn btn--accent" align="right">
            <ul className="menu__list">
              {userTypes.map((type) => (
                <li key={type}><Link to={`/${a.id}/user_invitations/new?type=${type}`}>{t(`management.types.${type}`)}</Link></li>
              ))}
            </ul>
          </Menu>
        )
      }
    >
      <table className="data-table" aria-label={t('management.parts.users')}>
        <thead>
          <tr>
            <th scope="col">{t('management.name')}</th>
            <th scope="col" className="data-table__extra">{t('management.email')}</th>
            <th scope="col">{t('management.access')}</th>
            <th scope="col" className="data-table__extra">{t('management.paymentRights')}</th>
          </tr>
        </thead>
        <tbody>
          {users.data?.map((u) => (
            <tr key={u.id} className={u.active ? undefined : 'is-inactive'}>
              <td>
                {owner ? <Link to={`/${a.id}/users/${u.id}`}>{u.name}</Link> : u.name}
                {!u.active && <span className="hint"> · {t('bookkeeping.inactive')}</span>}
              </td>
              <td className="data-table__extra">{u.email}</td>
              <td>{t(`management.types.${u.user_type ?? 'employee'}`)}</td>
              <td className="data-table__extra">-</td>
            </tr>
          ))}
        </tbody>
      </table>
      {owner && invitations.data && invitations.data.length > 0 && (
        <table className="data-table" aria-label={t('management.openInvitations')}>
          <thead>
            <tr>
              <th scope="col">{t('management.openInvitations')}</th>
              <th scope="col">{t('management.access')}</th>
              <th scope="col"><span className="sr-only">{t('bookkeeping.actions')}</span></th>
            </tr>
          </thead>
          <tbody>
            {invitations.data.map((inv) => (
              <tr key={inv.id}>
                <td>
                  {inv.email}
                  <p className="hint">{inv.expired ? t('management.expired') : t('management.expires', { date: date(inv.expires_at) })}</p>
                </td>
                <td>{t(`management.types.${inv.user_type}`)}</td>
                <td className="data-table__actions">
                  <button type="button" className="btn btn--secondary btn--small" disabled={revoke.isPending} aria-label={t('management.withdrawFor', { email: inv.email })} onClick={() => revoke.mutate(inv.id)}>
                    {t('management.withdraw')}
                  </button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </Block>
  )
}

function SecurityBlock({ administration: a }: { administration: Administration }) {
  const { t } = useTranslation()
  const me = useCurrentUser()
  const require = useRequireTwoFactor(a.id)
  const [confirming, setConfirming] = useState(false)
  const owner = can(a, 'ownership')
  const mine = me.data?.two_factor_enabled ?? false
  return (
    <Block id="security" title={t('management.parts.security')} subtitle={t('management.securitySubtitle')}>
      <div className="card card-stack">
        <SwitchRow
          label={t('management.requireTwoFactor')}
          help={a.require_two_factor ? t('management.requiredHelp') : t('management.requireHelp')}
          checked={a.require_two_factor || confirming}
          disabled={a.require_two_factor || !owner || require.isPending}
          onChange={(on) => setConfirming(on)}
        />
        {confirming && !a.require_two_factor && (
          <div className="confirm-row" role="alert">
            {mine ? (
              <>
                <p>{t('management.requireConfirm', { name: a.name })}</p>
                <div className="confirm-row__buttons">
                  <button type="button" className="btn btn--accent btn--small" disabled={require.isPending} onClick={() => require.mutate(undefined, { onSuccess: () => setConfirming(false) })}>{t('management.requireButton')}</button>
                  <button type="button" className="btn btn--secondary btn--small" onClick={() => setConfirming(false)}>{t('common.cancel')}</button>
                </div>
              </>
            ) : (
              <>
                <p>{t('management.setUpYoursFirst')}</p>
                <div className="confirm-row__buttons">
                  <Link className="btn btn--accent btn--small" to="/user#two-step">{t('management.setUpTwoFactor')}</Link>
                  <button type="button" className="btn btn--secondary btn--small" onClick={() => setConfirming(false)}>{t('common.cancel')}</button>
                </div>
              </>
            )}
          </div>
        )}
        <Problems messages={errorMessages(require.error, {})} />
      </div>
    </Block>
  )
}

function PrivacyBlock({ administration: a }: { administration: Administration }) {
  const { t, i18n } = useTranslation()
  const support = a.support_access_until && new Date(a.support_access_until) > new Date()
  const items = [
    { to: `/${a.id}/privacy/data_processing`, title: t('management.dpa'), help: t('management.dpaHelp'), state: null },
    can(a, 'ownership') || support
      ? {
          to: `/${a.id}/support_access`,
          title: t('management.supportAccess'),
          help: t('management.supportHelp'),
          state: support ? t('management.activeUntil', { date: new Date(a.support_access_until ?? '').toLocaleString(i18n.language, { dateStyle: 'medium', timeStyle: 'short' }) }) : t('management.off'),
        }
      : null,
    { to: `/${a.id}/privacy/data_analysis`, title: t('management.analysis'), help: t('management.analysisHelp'), state: t('management.off') },
  ].filter((i) => i !== null)
  return (
    <Block id="privacy" title={t('management.parts.privacy')} subtitle={t('management.privacySubtitle')}>
      <ul className="ledger">
        {items.map((i) => (
          <li className="ledger__row" key={i.to}>
            <div className="ledger__text">
              <Link className="ledger__name" to={i.to}>{i.title}</Link>
              <p className="hint">{i.help}</p>
            </div>
            {i.state && <span className="hint">{i.state}</span>}
          </li>
        ))}
      </ul>
    </Block>
  )
}

function LeaveBlock({ administration: a }: { administration: Administration }) {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const me = useCurrentUser()
  const users = useUsers(a.id)
  const leave = useLeave(a.id)
  const [confirming, setConfirming] = useState(false)
  const owners = users.data?.filter((u) => u.active && u.user_type === 'owner') ?? []
  const onlyOwner = a.user_type === 'owner' && owners.length <= 1 && owners.every((u) => u.id === me.data?.id)
  const support = a.access === 'support'
  return (
    <Block id="leave-or-delete" title={t('management.parts.leave')} subtitle={t('management.leaveSubtitle')}>
      <ul className="ledger">
        <li className="ledger__row">
          <div className="ledger__text">
            <span className="ledger__name">{t('management.leave')}</span>
            <p className="hint">{onlyOwner ? t('management.onlyOwner') : t('management.leaveHelp')}</p>
          </div>
          {!support && (confirming ? (
            <button type="button" className="btn btn--danger btn--small" disabled={leave.isPending} onClick={() => leave.mutate(undefined, { onSuccess: () => navigate('/administrations') })} autoFocus onBlur={() => setConfirming(false)}>
              {t('management.confirmLeave')}
            </button>
          ) : (
            <button type="button" className="btn btn--secondary btn--small" disabled={onlyOwner} onClick={() => setConfirming(true)}>{t('management.leaveButton')}</button>
          ))}
        </li>
        {can(a, 'ownership') && (
          <li className="ledger__row">
            <div className="ledger__text">
              <Link className="ledger__name ledger__name--danger" to={`/${a.id}/delete`}>{t('management.delete')}</Link>
              <p className="hint">{t('management.deleteHelp')}</p>
            </div>
          </li>
        )}
      </ul>
      <Problems messages={errorMessages(leave.error, {})} />
    </Block>
  )
}
