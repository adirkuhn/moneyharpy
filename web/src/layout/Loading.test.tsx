import { act, render, screen } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { Loading } from './Loading'

describe('Loading', () => {
  beforeEach(() => vi.useFakeTimers())
  afterEach(() => vi.useRealTimers())

  it('stays quiet for fast loads and then says it is loading', () => {
    render(<Loading />)
    expect(screen.queryByRole('status')).toBeNull()
    act(() => vi.advanceTimersByTime(400))
    expect(screen.getByRole('status')).toHaveTextContent('Loading…')
  })
})
