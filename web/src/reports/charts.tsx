import { type ReactNode, useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Bar, BarChart, CartesianGrid, Cell, Line, LineChart, Pie, PieChart, Tooltip, XAxis, YAxis } from 'recharts'
import { useAmount } from './ReportTable'

/**
 * The charts of the reports, after Moneybird's: thin bars and lines on a
 * recessive grid, one axis, the brand's chart colours in a fixed order
 * (--chart-1 to --chart-4, checked for colour blindness in light and dark),
 * and a tooltip on hover. The amounts always also show as text beside them.
 */

/** A series: what it is, its amount per category, and its place in the colour order. */
export type Series = { key: string; label: string; values: number[]; color: 1 | 2 | 3 | 4 | 5 }

export const chartColor = (n: number) => `var(--chart-${n})`

/** The width of the box a chart sits in, so it fills it; a fixed width where the browser cannot tell, as in tests. */
function useWidth() {
  const ref = useRef<HTMLDivElement>(null)
  const [width, setWidth] = useState(480)
  useEffect(() => {
    const el = ref.current
    if (!el || typeof ResizeObserver === 'undefined') {
      return
    }
    const observer = new ResizeObserver(([entry]) => setWidth(Math.max(200, Math.floor(entry?.contentRect.width ?? 480))))
    observer.observe(el)
    return () => observer.disconnect()
  }, [])
  return { ref, width }
}

/** Amounts on the axis, short: 12k, 1.2M. */
function useCompact() {
  const { i18n } = useTranslation()
  const f = new Intl.NumberFormat(i18n.language, { notation: 'compact', maximumFractionDigits: 1 })
  return (v: number) => f.format(v)
}

type TooltipProps = { active?: boolean; label?: ReactNode; payload?: { name?: ReactNode; value?: number | string; color?: string; dataKey?: string | number }[] }

/** The hover card: the category, and each series' amount beside its colour. */
function HoverCard({ active, label, payload }: TooltipProps) {
  const amount = useAmount()
  if (!active || !payload?.length) {
    return null
  }
  return (
    <div className="chart-tooltip">
      <p className="chart-tooltip__title">{label}</p>
      <ul>
        {payload.map((p) => (
          <li key={String(p.dataKey)}>
            <span className="chart-dot" style={{ background: p.color }} aria-hidden="true" />
            <span>{p.name}</span>
            <strong>{amount(Number(p.value ?? 0))}</strong>
          </li>
        ))}
      </ul>
    </div>
  )
}

const axis = { stroke: 'var(--line)', tick: { fill: 'var(--muted)', fontSize: 12 }, tickLine: false }

function rows(categories: string[], series: Series[]) {
  return categories.map((c, i) => Object.fromEntries([['category', c], ...series.map((s) => [s.key, s.values[i] ?? 0])]))
}

/** Bars per category, side by side per series. */
export function BarsChart({ label, categories, series, height = 220 }: { label: string; categories: string[]; series: Series[]; height?: number }) {
  const { ref, width } = useWidth()
  const compact = useCompact()
  return (
    <div ref={ref} className="chart" role="img" aria-label={label}>
      <BarChart width={width} height={height} data={rows(categories, series)} barGap={2} barCategoryGap="28%" margin={{ top: 8, right: 8, bottom: 0, left: 0 }}>
        <CartesianGrid vertical={false} stroke="var(--line)" strokeDasharray="0" />
        <XAxis dataKey="category" {...axis} axisLine={false} />
        <YAxis {...axis} axisLine={false} width={48} tickFormatter={compact} />
        <Tooltip content={<HoverCard />} cursor={{ fill: 'var(--row-hover)' }} />
        {series.map((s) => (
          <Bar key={s.key} dataKey={s.key} name={s.label} fill={chartColor(s.color)} radius={[4, 4, 0, 0]} maxBarSize={28} isAnimationActive={false} />
        ))}
      </BarChart>
    </div>
  )
}

/** A line per series over the categories, such as a balance at the end of each month. */
export function LinesChart({ label, categories, series, height = 220 }: { label: string; categories: string[]; series: Series[]; height?: number }) {
  const { ref, width } = useWidth()
  const compact = useCompact()
  return (
    <div ref={ref} className="chart" role="img" aria-label={label}>
      <LineChart width={width} height={height} data={rows(categories, series)} margin={{ top: 8, right: 12, bottom: 0, left: 0 }}>
        <CartesianGrid vertical={false} stroke="var(--line)" />
        <XAxis dataKey="category" {...axis} axisLine={false} />
        <YAxis {...axis} axisLine={false} width={48} tickFormatter={compact} />
        <Tooltip content={<HoverCard />} cursor={{ stroke: 'var(--muted)', strokeWidth: 1 }} />
        {series.map((s) => (
          <Line
            key={s.key}
            dataKey={s.key}
            name={s.label}
            stroke={chartColor(s.color)}
            strokeWidth={2}
            dot={{ r: 4, strokeWidth: 2, stroke: 'var(--surface)', fill: chartColor(s.color) }}
            activeDot={{ r: 5 }}
            isAnimationActive={false}
          />
        ))}
      </LineChart>
    </div>
  )
}

/** Parts of a whole, such as what each customer owes, as a ring. */
export function RingChart({ label, parts, height = 220 }: { label: string; parts: { key: string; label: string; value: number; color: number }[]; height?: number }) {
  const { ref, width } = useWidth()
  const shown = parts.filter((p) => p.value > 0)
  return (
    <div ref={ref} className="chart" role="img" aria-label={label}>
      <PieChart width={width} height={height}>
        <Tooltip content={<HoverCard />} />
        <Pie
          data={shown.length > 0 ? shown : [{ key: 'none', label: '', value: 1, color: 0 }]}
          dataKey="value"
          nameKey="label"
          innerRadius={height / 2 - 44}
          outerRadius={height / 2 - 16}
          paddingAngle={shown.length > 1 ? 1 : 0}
          stroke="var(--surface)"
          strokeWidth={2}
          isAnimationActive={false}
        >
          {(shown.length > 0 ? shown : [{ key: 'none', color: 0 }]).map((p) => (
            <Cell key={p.key} fill={p.color === 0 ? 'var(--chart-other)' : chartColor(p.color)} />
          ))}
        </Pie>
      </PieChart>
    </div>
  )
}

/** The legend under a chart: a dot in the series' colour, its name and its amount. */
export function Legend({ items }: { items: { key: string; label: ReactNode; value: string; color?: number }[] }) {
  return (
    <ul className="chart-legend">
      {items.map((i) => (
        <li key={i.key}>
          <span className="chart-dot" style={{ background: i.color ? chartColor(i.color) : 'var(--chart-other)' }} aria-hidden="true" />
          <span className="chart-legend__label">{i.label}</span>
          <span className="chart-legend__value">{i.value}</span>
        </li>
      ))}
    </ul>
  )
}
