import { useTranslation } from 'react-i18next'
import { CheckboxInput, FormSection } from '../forms/Form'
import { type Permission, permissions, type UserType } from './queries'

type Props = { type: UserType; value: Permission[]; onChange: (p: Permission[]) => void }

/** Checkboxes for what a user may do. Owners hold everything; ownership comes only with the owner type. */
export function PermissionFields({ type, value, onChange }: Props) {
  const { t } = useTranslation()
  return (
    <FormSection title={t('management.permissions')}>
      {permissions.filter((p) => p !== 'ownership').map((p) => (
        <CheckboxInput
          key={p}
          label={t(`management.permissionNames.${p}`)}
          help={t(`management.permissionHelp.${p}`)}
          checked={type === 'owner' || value.includes(p)}
          disabled={type === 'owner'}
          onChange={(e) => onChange(e.target.checked ? [...value, p] : value.filter((x) => x !== p))}
        />
      ))}
      {type === 'owner' && <p className="hint">{t('management.ownerHasAll')}</p>}
    </FormSection>
  )
}
