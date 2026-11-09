import createClient from 'openapi-fetch'
import type { components, paths } from './schema'

export type Administration = components['schemas']['Administration']
export type CurrentUser = components['schemas']['CurrentUser']
export type ApiToken = components['schemas']['ApiToken']
export type InstanceSettings = components['schemas']['InstanceSettings']
export type UpdateInstanceSettings = components['schemas']['UpdateInstanceSettingsRequest']
export type Language = components['schemas']['Language']

export const api = createClient<paths>({
  baseUrl: window.location.origin,
  // Resolved per call so tests can intercept the global fetch.
  fetch: (request) => globalThis.fetch(request),
})

/** A failed API call, carrying the HTTP status and per-field problems. */
export class ApiError extends Error {
  readonly status: number
  readonly fields: Record<string, string[]>

  constructor(status: number, body: unknown) {
    const error = (body as { error?: unknown } | undefined)?.error
    super(typeof error === 'string' ? error : 'The request failed')
    this.status = status
    this.fields = typeof error === 'object' && error !== null ? (error as Record<string, string[]>) : {}
  }
}

type Result<T> = { data?: T; error?: unknown; response: Response }

/** Returns the body of a successful call and throws ApiError otherwise. */
export async function unwrap<T>(call: Promise<Result<T>>): Promise<T> {
  const { data, error, response } = await call
  if (!response.ok) {
    throw new ApiError(response.status, error)
  }
  return data as T
}

/** Turns an error into sentences, using labels for the field names. */
export function errorMessages(error: unknown, labels: Record<string, string>): string[] {
  if (!(error instanceof ApiError)) {
    return error ? ['Something went wrong. Try again.'] : []
  }
  const fields = Object.entries(error.fields)
  if (fields.length === 0) {
    return [error.message]
  }
  return fields.flatMap(([field, problems]) => problems.map((p) => `${labels[field] ?? field} ${p}`))
}
