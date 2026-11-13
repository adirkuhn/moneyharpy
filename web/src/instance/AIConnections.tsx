import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import type { components } from '../api/schema'
import { SelectInput, TextInput } from '../forms/Form'

type Provider = components['schemas']['RecognitionAIProvider']
type Stored = components['schemas']['RecognitionAIConnection']
type Request = components['schemas']['RecognitionAIConnectionRequest']

/** A connection being edited: a stored one (with id) or a new one; apiKey is a newly typed key. */
export type DraftConnection = Omit<Stored, 'id'> & { id?: string; key: string; apiKey: string }

/** The default model of each provider, shown as the model's placeholder. */
const defaultModels: Record<Provider, string> = {
  ollama: 'qwen2.5vl',
  anthropic: 'claude-opus-5-5',
  nvidia: 'meta/llama-3.2-90b-vision-instruct',
}

const providerNames: Record<Provider, string> = { ollama: 'Ollama', anthropic: 'Anthropic', nvidia: 'NVIDIA' }

let nextKey = 0
const newKey = () => `new-${++nextKey}`

export function toDrafts(stored: Stored[]): DraftConnection[] {
  return stored.map((c) => ({ ...c, key: c.id, apiKey: '' }))
}

/** The connections as the update request sends them: a key only when one was typed. */
export function toRequest(drafts: DraftConnection[]): Request[] {
  return drafts.map(({ id, name, provider, url, model, enabled, apiKey }) => ({
    ...(id ? { id } : {}),
    name,
    provider,
    url,
    model,
    enabled,
    ...(apiKey ? { api_key: apiKey } : {}),
  }))
}

/** Field labels for server problems such as "recognition_ai_connections[0].api_key". */
export function connectionLabels(drafts: DraftConnection[], t: (key: string) => string): Record<string, string> {
  const fields = { name: t('instance.aiName'), provider: t('instance.aiProvider'), url: t('instance.aiUrlLabel'), model: t('instance.aiModel'), api_key: t('instance.aiKey') }
  const labels: Record<string, string> = { recognition_ai_connections: t('instance.aiConnections') }
  drafts.forEach((_, i) => {
    for (const [field, label] of Object.entries(fields)) {
      labels[`recognition_ai_connections[${i}].${field}`] = label
    }
  })
  return labels
}

type Props = {
  connections: DraftConnection[]
  onChange: (connections: DraftConnection[]) => void
  /** Problems the server reported for a field, such as "recognition_ai_connections[0].api_key". */
  errors: (field: string) => string[] | undefined
}

/** The AI connections, in the order Scan and Recognise asks them, with an editor for one. */
export function AIConnections({ connections, onChange, errors }: Props) {
  const { t } = useTranslation()
  const [editing, setEditing] = useState<DraftConnection | null>(null)

  const move = (index: number, by: number) => {
    const next = [...connections]
    const [moved] = next.splice(index, 1)
    if (moved) next.splice(index + by, 0, moved)
    onChange(next)
  }
  const update = (index: number, change: Partial<DraftConnection>) =>
    onChange(connections.map((c, i) => (i === index ? { ...c, ...change } : c)))
  const done = (draft: DraftConnection) => {
    const exists = connections.some((c) => c.key === draft.key)
    onChange(exists ? connections.map((c) => (c.key === draft.key ? draft : c)) : [...connections, draft])
    setEditing(null)
  }
  const add = () =>
    setEditing({ key: newKey(), name: providerNames.ollama, provider: 'ollama', url: '', model: '', api_key_set: false, enabled: true, apiKey: '' })

  return (
    <div className="form-input">
      <span className="form-input__label">{t('instance.aiConnections')}</span>
      <div className="form-input__body ai-connections">
        <p className="hint">{t('instance.aiConnectionsHint')}</p>
        {connections.length === 0 ? (
          <p>{t('instance.aiNoConnections')}</p>
        ) : (
          <ol className="ai-connections__list" aria-label={t('instance.aiConnections')}>
            {connections.map((c, index) => {
              const problems = ['name', 'provider', 'url', 'model', 'api_key'].flatMap((f) => errors(`recognition_ai_connections[${index}].${f}`) ?? [])
              return (
                <li key={c.key} className="ai-connections__item">
                  <label className="check">
                    <input type="checkbox" checked={c.enabled} aria-label={t('instance.aiUse', { name: c.name })} onChange={(e) => update(index, { enabled: e.target.checked })} />
                    <span>
                      <strong>{c.name}</strong> <span className="ai-connections__meta">{providerNames[c.provider]} · {c.model || t('instance.aiDefaultModel')}</span>
                    </span>
                  </label>
                  <span className="ai-connections__actions">
                    <button type="button" className="icon-btn" aria-label={t('instance.aiMoveUp', { name: c.name })} disabled={index === 0} onClick={() => move(index, -1)}>↑</button>
                    <button type="button" className="icon-btn" aria-label={t('instance.aiMoveDown', { name: c.name })} disabled={index === connections.length - 1} onClick={() => move(index, 1)}>↓</button>
                    <button type="button" className="btn btn--secondary btn--small" aria-label={t('instance.aiEdit', { name: c.name })} onClick={() => setEditing(c)}>{t('settings.edit')}</button>
                    <button type="button" className="btn btn--secondary btn--small" aria-label={t('instance.aiRemove', { name: c.name })} onClick={() => onChange(connections.filter((_, i) => i !== index))}>{t('instance.aiRemoveText')}</button>
                  </span>
                  {problems.length > 0 && <p className="recognition-check__problem">{problems.join(' ')}</p>}
                </li>
              )
            })}
          </ol>
        )}
        {editing ? (
          <ConnectionEditor
            draft={editing}
            isNew={!connections.some((c) => c.key === editing.key)}
            onDone={done}
            onCancel={() => setEditing(null)}
          />
        ) : (
          <p>
            <button type="button" className="btn btn--secondary" disabled={connections.length >= 10} onClick={add}>{t('instance.aiAdd')}</button>
          </p>
        )}
      </div>
    </div>
  )
}

type EditorProps = { draft: DraftConnection; isNew: boolean; onDone: (draft: DraftConnection) => void; onCancel: () => void }

function ConnectionEditor({ draft: initial, isNew, onDone, onCancel }: EditorProps) {
  const { t } = useTranslation()
  const [draft, setDraft] = useState(initial)
  const set = (change: Partial<DraftConnection>) => setDraft({ ...draft, ...change })
  const changeProvider = (provider: Provider) =>
    // A name that was only the provider's follows the new provider.
    set({ provider, url: '', ...(draft.name === providerNames[draft.provider] || draft.name === '' ? { name: providerNames[provider] } : {}) })
  const title = isNew ? t('instance.aiNewConnection') : t('instance.aiEdit', { name: initial.name })

  return (
    <fieldset className="ai-connections__editor" aria-label={title}>
      <legend>{title}</legend>
      <TextInput label={t('instance.aiName')} width="medium" autoComplete="off" value={draft.name} onChange={(e) => set({ name: e.target.value })} />
      <SelectInput label={t('instance.aiProvider')} width="medium" value={draft.provider} onChange={(e) => changeProvider(e.target.value as Provider)}>
        <option value="ollama">{t('instance.aiOllama')}</option>
        <option value="anthropic">{t('instance.aiAnthropic')}</option>
        <option value="nvidia">{t('instance.aiNvidia')}</option>
      </SelectInput>
      {draft.provider === 'nvidia' && <p className="hint">{t('instance.aiNvidiaNote')}</p>}
      {draft.provider === 'ollama' && (
        <TextInput label={t('instance.aiUrl')} type="url" autoComplete="off" placeholder="http://ollama:11434" value={draft.url} onChange={(e) => set({ url: e.target.value })} />
      )}
      {draft.provider === 'nvidia' && (
        <TextInput
          label={t('instance.aiUrlLabel')}
          type="url"
          autoComplete="off"
          placeholder="https://integrate.api.nvidia.com/v1"
          help={t('instance.aiNvidiaUrlHelp')}
          value={draft.url}
          onChange={(e) => set({ url: e.target.value })}
        />
      )}
      <TextInput label={t('instance.aiModel')} width="medium" autoComplete="off" placeholder={defaultModels[draft.provider]} value={draft.model} onChange={(e) => set({ model: e.target.value })} />
      {draft.provider !== 'ollama' && (
        <TextInput
          label={t('instance.aiKey')}
          type="password"
          autoComplete="off"
          placeholder={draft.api_key_set ? t('instance.stored') : ''}
          value={draft.apiKey}
          onChange={(e) => set({ apiKey: e.target.value })}
        />
      )}
      <div className="form__actions">
        <button type="button" className="btn btn--secondary" onClick={() => onDone(draft)}>{t('instance.aiDone')}</button>
        <button type="button" className="btn btn--secondary" onClick={onCancel}>{t('common.cancel')}</button>
      </div>
    </fieldset>
  )
}
