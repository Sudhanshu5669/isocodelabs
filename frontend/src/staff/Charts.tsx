import { useEffect, useRef, useState } from 'react'
import { day } from '../lib/format'

// Two small hand-made charts for the dashboard. No chart library: they are a
// few dozen lines each, and this keeps them in the app's own type and colours.
// Rules followed: one series in one colour (so no legend box; the heading says
// what is plotted), thin bars with a rounded end and a square base, hairline
// grid, a hover/focus tooltip, and a table with the same numbers for anyone
// who cannot use the chart.

/** Width of an element, kept up to date as the window resizes. */
function useWidth<T extends HTMLElement>() {
  const ref = useRef<T>(null)
  const [width, setWidth] = useState(0)
  useEffect(() => {
    const el = ref.current
    if (!el) return
    const ro = new ResizeObserver(([e]) => setWidth(e.contentRect.width))
    ro.observe(el)
    return () => ro.disconnect()
  }, [])
  return [ref, width] as const
}

/** A round number at or above n, for the top of an axis: 1, 2, 5, 10, 20, 50 ... */
function niceMax(n: number) {
  if (n <= 0) return 1
  const p = 10 ** Math.floor(Math.log10(n))
  for (const m of [1, 2, 5, 10]) if (m * p >= n) return m * p
  return 10 * p
}

/** A column whose top corners are rounded and whose base is square. */
function columnPath(x: number, y: number, w: number, h: number, r = 4) {
  const rr = Math.min(r, w / 2, h)
  return `M${x},${y + h}V${y + rr}Q${x},${y} ${x + rr},${y}H${x + w - rr}Q${x + w},${y} ${x + w},${y + rr}V${y + h}Z`
}

/** "1 lead", "3 leads" */
const plural = (n: number, unit: string) => (n === 1 ? unit.replace(/s$/, '') : unit)

export type DayPoint = { date: string; value: number; note?: string }

/** Columns, one per day. */
export function DailyColumns({ points, unit }: { points: DayPoint[]; unit: string }) {
  const [ref, width] = useWidth<HTMLDivElement>()
  const [hover, setHover] = useState<number | null>(null)

  const height = 200
  const pad = { top: 12, right: 8, bottom: 26, left: 36 }
  const plotW = Math.max(0, width - pad.left - pad.right)
  const plotH = height - pad.top - pad.bottom
  const max = niceMax(Math.max(...points.map((p) => p.value), 1))
  const band = points.length ? plotW / points.length : 0
  const barW = Math.min(24, Math.max(1, band - 2)) // at least a 2px gap between columns
  const y = (v: number) => pad.top + plotH - (v / max) * plotH
  const ticks = [0, max / 2, max]
  const labelAt = new Set([0, Math.floor((points.length - 1) / 2), points.length - 1])

  const h = hover != null ? points[hover] : null

  return (
    <div className="chart" ref={ref}>
      {width > 0 && (
        <svg width={width} height={height} role="img" aria-label={`${unit} per day`} onMouseLeave={() => setHover(null)}>
          {ticks.map((t) => (
            <g key={t}>
              <line x1={pad.left} x2={width - pad.right} y1={y(t)} y2={y(t)} className="chart-grid" />
              <text x={pad.left - 8} y={y(t)} dy="0.32em" textAnchor="end" className="chart-tick">
                {Number.isInteger(t) ? t : t.toFixed(1)}
              </text>
            </g>
          ))}
          {points.map((p, i) => {
            const cx = pad.left + i * band + band / 2
            const bh = (p.value / max) * plotH
            return (
              <g key={p.date}>
                {bh > 0 && (
                  <path
                    d={columnPath(cx - barW / 2, y(p.value), barW, bh)}
                    className={`chart-bar ${hover === i ? 'is-hover' : ''}`}
                  />
                )}
                {labelAt.has(i) && (
                  <text
                    x={cx}
                    y={height - 6}
                    textAnchor={i === 0 ? 'start' : i === points.length - 1 ? 'end' : 'middle'}
                    className="chart-tick"
                  >
                    {day(p.date + 'T12:00:00+05:30')}
                  </text>
                )}
                {/* The hover target is the whole day's band, taller and wider than the bar. */}
                <rect
                  x={pad.left + i * band}
                  y={pad.top}
                  width={band}
                  height={plotH}
                  fill="transparent"
                  tabIndex={0}
                  aria-label={`${day(p.date + 'T12:00:00+05:30')}: ${p.value} ${plural(p.value, unit)}`}
                  onMouseEnter={() => setHover(i)}
                  onFocus={() => setHover(i)}
                  onBlur={() => setHover(null)}
                />
              </g>
            )
          })}
        </svg>
      )}
      {h && hover != null && (
        <div
          className="chart-tip"
          style={{
            left: Math.min(Math.max(pad.left + hover * band + band / 2, 70), width - 70),
            top: y(h.value) - 8,
          }}
        >
          <strong>{day(h.date + 'T12:00:00+05:30')}</strong>
          <span className="num">
            {h.value} {plural(h.value, unit)}
          </span>
          {h.note && <span className="muted">{h.note}</span>}
        </div>
      )}
    </div>
  )
}

function barWidth(share: number) {
  const pct = Math.max(0.01, share) * 100
  return `calc(${pct}% - ${pct * 0.96}px)`
}

export type BarRow ={ label: string; value: number | null; display: string; note?: string }

/** Horizontal bars with the value at the tip. Rows with no value say why instead of drawing a bar. */
export function HorizontalBars({ rows }: { rows: BarRow[] }) {
  const max = Math.max(...rows.map((r) => r.value ?? 0), 1)
  return (
    <ul className="hbars">
      {rows.map((r) => (
        <li key={r.label} className="hbar-row">
          <span className="hbar-label">{r.label}</span>
          <span className="hbar-track">
            {r.value != null ? (
              <>
                {/* Scaled to the track minus 96px, so the value label always fits beside the longest bar. */}
                <span className="hbar" style={{ width: barWidth(r.value / max) }} />
                <span className="hbar-value num">{r.display}</span>
              </>
            ) : (
              <span className="hbar-value muted">{r.note ?? '–'}</span>
            )}
          </span>
        </li>
      ))}
    </ul>
  )
}
