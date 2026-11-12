import { type ReactNode, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Problems } from '../auth/AuthLayout'
import { type FormErrors, SelectInput, TextArea, TextInput } from '../forms/Form'
import { AccordionItem, PartIcon } from './Accordion'
import type { ChannelForm } from './ChannelPreviews'
import type { DocumentStyle } from './queries'
import { AlignIcon, ColorField, Range, Segmented, Toggle } from './StyleControls'
import { StyleFileField } from './StyleFileField'

export const fonts = ['lucida_grande', 'arial', 'helvetica', 'verdana', 'georgia', 'times_new_roman', 'courier'] as const
export const headerTypes = ['', 'background_image_with_text', 'background_image_with_logo', 'background_image', 'background_color_with_logo', 'background_color_with_text'] as const
export const footerTypes = ['', 'contact_information', 'signature', 'background_image'] as const
export const publicViewColors = ['#F7F9FC', '#F6E4E2', '#EDE8F7', '#D5E8FA', '#DEF4F8', '#FBE0D5', '#FCF1DB', '#D7ECEA', '#E7F3E6', '#E4E9EB'] as const

type Icon = Parameters<typeof PartIcon>[0]['kind']
export type PartSpec = { part: string; icon: Icon; children?: ReactNode; disabled?: boolean; tip?: string; fields?: string[] }

/** Moneybird's parts in a column; a part with a refused field opens by itself. */
export function Parts({ items, errors, general }: { items: PartSpec[]; errors: FormErrors; general?: ReactNode }) {
  const { t } = useTranslation()
  const [open, setOpen] = useState<Set<string>>(new Set())
  return (
    <div className="style-accordion">
      <Problems messages={errors.general} />
      {general}
      {items.map(({ part, icon, children, disabled, tip, fields = [] }) => (
        <AccordionItem
          key={part}
          title={t(`documentStyle.sections.${part}`)}
          icon={<PartIcon kind={icon} />}
          open={open.has(part) || fields.some((f) => errors.field(f))}
          disabled={disabled}
          tip={tip}
          onToggle={() => {
            const next = new Set(open)
            if (!next.delete(part)) {
              next.add(part)
            }
            setOpen(next)
          }}
        >
          {children}
        </AccordionItem>
      ))}
    </div>
  )
}

type Props = {
  administrationId: string
  style: DocumentStyle
  onFile: (style: DocumentStyle) => void
  form: ChannelForm
  set: <K extends keyof ChannelForm>(key: K, value: ChannelForm[K]) => void
  errors: FormErrors
}

/** The Email tab: the header, footer and general look of the emails documents go out with. */
export function EmailParts({ administrationId, style, onFile, form, set, errors }: Props) {
  const { t } = useTranslation()
  const header = form.email_header_type
  const footer = form.email_footer_type
  const withImage = header.startsWith('background_image')
  const withColor = header.startsWith('background_color')
  return (
    <Parts
      errors={errors}
      items={[
        {
          part: 'header', icon: 'mail',
          fields: ['email_header_type', 'email_header_content_position', 'email_header_background_color', 'email_header_text', 'email_header_text_color', 'email_logo_display_width'],
          children: (
            <>
              <SelectInput label={t('documentStyle.email.headerType')} value={header} errors={errors.field('email_header_type')} onChange={(e) => set('email_header_type', e.target.value as ChannelForm['email_header_type'])}>
                {headerTypes.map((v) => <option key={v} value={v}>{t(`documentStyle.email.headerTypes.${v || 'none'}`)}</option>)}
              </SelectInput>
              {header !== '' && header !== 'background_image' && (
                <Segmented
                  label={t('documentStyle.email.contentPosition')}
                  value={form.email_header_content_position}
                  options={(['left', 'center', 'right'] as const).map((v) => ({ value: v, label: t(`documentStyle.positions.${v}`), icon: <AlignIcon align={v} /> }))}
                  errors={errors.field('email_header_content_position')}
                  onChange={(v) => set('email_header_content_position', v)}
                />
              )}
              {withColor && <ColorField label={t('documentStyle.email.backgroundColor')} value={form.email_header_background_color} errors={errors.field('email_header_background_color')} onChange={(v) => set('email_header_background_color', v)} />}
              {header.endsWith('_with_text') && (
                <>
                  <TextArea label={t('documentStyle.email.text')} rows={3} maxLength={500} value={form.email_header_text} errors={errors.field('email_header_text')} onChange={(e) => set('email_header_text', e.target.value)} />
                  <ColorField label={t('documentStyle.email.textColor')} value={form.email_header_text_color} errors={errors.field('email_header_text_color')} onChange={(v) => set('email_header_text_color', v)} />
                </>
              )}
              {header.endsWith('_with_logo') && (
                <>
                  <StyleFileField administrationId={administrationId} style={style} kind="email_logo" onChange={onFile} />
                  <Range label={t('documentStyle.logoWidth')} value={form.email_logo_display_width} min={10} max={100} unit="%" errors={errors.field('email_logo_display_width')} onChange={(v) => set('email_logo_display_width', v)} />
                </>
              )}
              {withImage && <StyleFileField administrationId={administrationId} style={style} kind="email_header_background" onChange={onFile} />}
            </>
          ),
        },
        {
          part: 'footer', icon: 'mail',
          fields: ['email_footer_type', 'email_footer_text_color', 'email_footer_background_color', 'email_footer_profile_title', 'email_footer_profile_subtitle'],
          children: (
            <>
              <SelectInput label={t('documentStyle.email.footerType')} value={footer} errors={errors.field('email_footer_type')} onChange={(e) => set('email_footer_type', e.target.value as ChannelForm['email_footer_type'])}>
                {footerTypes.map((v) => <option key={v} value={v}>{t(`documentStyle.email.footerTypes.${v || 'none'}`)}</option>)}
              </SelectInput>
              {(footer === 'contact_information' || footer === 'signature') && (
                <>
                  <ColorField label={t('documentStyle.email.textColor')} value={form.email_footer_text_color} errors={errors.field('email_footer_text_color')} onChange={(v) => set('email_footer_text_color', v)} />
                  <ColorField label={t('documentStyle.email.backgroundColor')} value={form.email_footer_background_color} errors={errors.field('email_footer_background_color')} onChange={(v) => set('email_footer_background_color', v)} />
                </>
              )}
              {footer === 'signature' && (
                <>
                  <TextInput label={t('documentStyle.email.title')} maxLength={100} value={form.email_footer_profile_title} errors={errors.field('email_footer_profile_title')} onChange={(e) => set('email_footer_profile_title', e.target.value)} />
                  <TextInput label={t('documentStyle.email.subtitle')} maxLength={100} value={form.email_footer_profile_subtitle} errors={errors.field('email_footer_profile_subtitle')} onChange={(e) => set('email_footer_profile_subtitle', e.target.value)} />
                  <StyleFileField administrationId={administrationId} style={style} kind="email_footer_profile_picture" onChange={onFile} />
                </>
              )}
              {footer === 'background_image' && <StyleFileField administrationId={administrationId} style={style} kind="email_footer_background" onChange={onFile} />}
            </>
          ),
        },
        {
          part: 'general', icon: 'cog', fields: ['body_background_color', 'email_font_family'],
          children: (
            <>
              <ColorField label={t('documentStyle.email.backgroundColor')} value={form.body_background_color} errors={errors.field('body_background_color')} onChange={(v) => set('body_background_color', v)} />
              <SelectInput label={t('documentStyle.fontFamily')} value={form.email_font_family} errors={errors.field('email_font_family')} onChange={(e) => set('email_font_family', e.target.value as ChannelForm['email_font_family'])}>
                {fonts.map((f) => <option key={f} value={f}>{t(`documentStyle.fonts.${f}`)}</option>)}
              </SelectInput>
              <Toggle label={t('documentStyle.label')} tip={t('documentStyle.email.labelHelp')} checked={form.email_moneybird_label_enabled} onChange={(on) => set('email_moneybird_label_enabled', on)} />
            </>
          ),
        },
      ]}
    />
  )
}

/** The Public views tab: how the online view of documents looks. */
export function PublicViewParts({ administrationId, style, onFile, form, set, errors }: Props) {
  const { t } = useTranslation()
  const elsewhere = t('documentStyle.onlyUnderDocument')
  return (
    <Parts
      errors={errors}
      items={[
        {
          part: 'logo', icon: 'image', fields: ['public_view_logo_display_size'],
          children: (
            <>
              <StyleFileField administrationId={administrationId} style={style} kind="public_view_logo" onChange={onFile} />
              <Range label={t('documentStyle.logoSize')} value={form.public_view_logo_display_size} min={10} max={100} unit="%" errors={errors.field('public_view_logo_display_size')} onChange={(v) => set('public_view_logo_display_size', v)} />
            </>
          ),
        },
        {
          part: 'general', icon: 'cog', fields: ['public_view_background_color'],
          children: (
            <>
              <SelectInput label={t('documentStyle.email.backgroundColor')} value={form.public_view_background_color} errors={errors.field('public_view_background_color')} onChange={(e) => set('public_view_background_color', e.target.value as ChannelForm['public_view_background_color'])}>
                {publicViewColors.map((c, i) => <option key={c} value={c}>{t(`documentStyle.colors.${i}`)}</option>)}
              </SelectInput>
              <Toggle label={t('documentStyle.label')} tip={t('documentStyle.publicLabelHelp')} checked={form.public_view_moneybird_label_enabled} onChange={(on) => set('public_view_moneybird_label_enabled', on)} />
            </>
          ),
        },
        ...(['invoiceSender', 'estimateSender', 'invoiceRecipient', 'estimateRecipient'] as const).map((part) => ({ part, icon: 'card' as const, disabled: true, tip: elsewhere })),
      ]}
    />
  )
}

/** The Online sales tab: what the checkout asks and shows. Its logo is the public view's. */
export function CheckoutParts({ form, set, errors }: Props) {
  const { t } = useTranslation()
  const toggle = (key: 'checkout_show_address' | 'checkout_show_phone_field' | 'checkout_for_business' | 'checkout_show_identity' | 'checkout_show_withdrawal_button', tip?: string) => (
    <Toggle label={t(`documentStyle.checkout.${key}`)} tip={tip} checked={form[key]} onChange={(on) => set(key, on)} />
  )
  return (
    <Parts
      errors={errors}
      items={[
        { part: 'logo', icon: 'image', disabled: true, tip: t('documentStyle.checkout.logoTip') },
        {
          part: 'basic', icon: 'cog',
          children: (
            <>
              {toggle('checkout_show_address')}
              {toggle('checkout_show_phone_field')}
              {toggle('checkout_for_business', t('documentStyle.checkout.businessHelp'))}
              {toggle('checkout_show_identity')}
              {toggle('checkout_show_withdrawal_button', t('documentStyle.checkout.withdrawalHelp'))}
            </>
          ),
        },
        {
          part: 'favorite', icon: 'cog', fields: ['checkout_favorite_label'],
          children: (
            <TextArea
              label={t('documentStyle.checkout.favoriteLabel')}
              help={`${t('documentStyle.checkout.favoriteHelp')} ${form.checkout_favorite_label.length}/20`}
              rows={1}
              maxLength={20}
              value={form.checkout_favorite_label}
              errors={errors.field('checkout_favorite_label')}
              onChange={(e) => set('checkout_favorite_label', e.target.value)}
            />
          ),
        },
      ]}
    />
  )
}
