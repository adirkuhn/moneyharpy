import { type PointerEvent, useEffect, useRef } from 'react'
import { useTranslation } from 'react-i18next'
import './signature.css'

/** What was drawn: the signature as a PNG data URL, and its strokes as [x, y, ms] points, kept as evidence. */
export type Signature = { image: string; strokes: number[][][] }

type Props = {
  label: string
  /** Called with the signature after each stroke, or null once cleared. */
  onChange: (signature: Signature | null) => void
  invalid?: boolean
}

/**
 * A field to sign in with mouse, pen or finger, after Moneybird's signature
 * pad: the line to sign on and a link to clear it.
 */
export function SignaturePad({ label, onChange, invalid = false }: Props) {
  const { t } = useTranslation()
  const canvas = useRef<HTMLCanvasElement>(null)
  const strokes = useRef<number[][][]>([])
  const drawing = useRef(false)
  const started = useRef(0)

  // The drawing surface follows the field's width at the screen's density, so lines stay sharp.
  useEffect(() => {
    const c = canvas.current
    if (!c) {
      return
    }
    const ratio = window.devicePixelRatio || 1
    c.width = Math.max(1, Math.round(c.clientWidth * ratio))
    c.height = Math.max(1, Math.round(c.clientHeight * ratio))
    const ctx = c.getContext('2d')
    ctx?.scale(ratio, ratio)
  }, [])

  const point = (e: PointerEvent<HTMLCanvasElement>) => {
    const box = e.currentTarget.getBoundingClientRect()
    return [Math.round(e.clientX - box.left), Math.round(e.clientY - box.top), Math.round(e.timeStamp - started.current)]
  }
  const context = () => {
    const ctx = canvas.current?.getContext('2d')
    if (ctx) {
      ctx.lineWidth = 2
      ctx.lineCap = 'round'
      ctx.lineJoin = 'round'
      ctx.strokeStyle = '#111827'
    }
    return ctx
  }
  const down = (e: PointerEvent<HTMLCanvasElement>) => {
    e.preventDefault()
    e.currentTarget.setPointerCapture?.(e.pointerId)
    if (strokes.current.length === 0) {
      started.current = e.timeStamp
    }
    drawing.current = true
    const p = point(e)
    strokes.current.push([p])
    const ctx = context()
    ctx?.beginPath()
    ctx?.moveTo(p[0] ?? 0, p[1] ?? 0)
  }
  const move = (e: PointerEvent<HTMLCanvasElement>) => {
    if (!drawing.current) {
      return
    }
    const p = point(e)
    strokes.current[strokes.current.length - 1]?.push(p)
    const ctx = context()
    ctx?.lineTo(p[0] ?? 0, p[1] ?? 0)
    ctx?.stroke()
  }
  const up = () => {
    if (!drawing.current) {
      return
    }
    drawing.current = false
    onChange({ image: canvas.current?.toDataURL('image/png') ?? '', strokes: strokes.current })
  }
  const clear = () => {
    const c = canvas.current
    c?.getContext('2d')?.clearRect(0, 0, c.width, c.height)
    strokes.current = []
    onChange(null)
  }

  return (
    <div className={`signature-pad${invalid ? ' signature-pad--invalid' : ''}`}>
      <span className="signature-pad__label">{label}</span>
      <canvas
        ref={canvas}
        className="signature-pad__canvas"
        role="img"
        aria-label={label}
        onPointerDown={down}
        onPointerMove={move}
        onPointerUp={up}
        onPointerLeave={up}
      />
      <button type="button" className="btn btn--inline signature-pad__clear" onClick={clear}>{t('signature.clear')}</button>
    </div>
  )
}
