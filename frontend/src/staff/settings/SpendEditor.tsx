import { useState } from 'react'
import { api } from '../../api/client'
import type { Source, Spend } from '../../api/types'
import { ErrorState, Loading, Notice } from '../../components/ui'
import { isoDay, rupees } from '../../lib/format'
import { sourceLabel } from '../../lib/labels'
import { errorText, useApi } from '../../lib/useApi'

// What each paid source cost per month. The dashboard divides this by the
// visits each source produced to get cost per visit.

const SOURCES: Source[] = ['property_portal', 'social_ad', 'website', 'hoarding_call']

/** The last six months, oldest first, as "2026-11". */
function lastMonths(n = 6): string[] {
  const [y, m] = isoDay(new Date()).split('-').map(Number)
  return Array.from({ length: n }, (_, i) => {
    const d = new Date(Date.UTC(y, m - 1 - (n - 1 - i), 1))
    return d.toISOString().slice(0, 7)
  })
}

const monthName = (ym: string) =>
  new Date(ym + '-01T00:00:00Z').toLocaleDateString('en-IN', { month: 'short', year: 'numeric', timeZone: 'UTC' })

export default function SpendEditor() {
  const months = lastMonths()
  const res = useApi<{ spend: Spend[] }>(`/staff/spend?from=${months[0]}&to=${months[months.length - 1]}`)
  const [edits, setEdits] = useState<Record<string, string>>({}) // "source|month" -> typed amount
  const [busy, setBusy] = useState(false)
  const [notice, setNotice] = useState('')
  const [error, setError] = useState('')

  if (res.error) return <ErrorState message={res.error} onRetry={res.reload} />
  if (!res.data) return <Loading label="Loading spend" />

  const saved: Record<string, number> = {}
  for (const s of res.data.spend) saved[`${s.source}|${s.month.slice(0, 7)}`] = s.amountInr

  const changed = Object.entries(edits).filter(([k, v]) => v.trim() !== '' && Number(v) !== saved[k])
  const invalid = changed.some(([, v]) => !/^\d{1,11}$/.test(v.trim()))

  async function save() {
    setBusy(true)
    setError('')
    setNotice('')
    let done = 0
    try {
      for (const [k, v] of changed) {
        const [source, month] = k.split('|')
        await api('/staff/spend', { method: 'PUT', body: { source, month, amountInr: Number(v) } })
        done++
      }
      setEdits({})
      setNotice(`Saved ${done} change${done === 1 ? '' : 's'}. The dashboard uses them straight away.`)
    } catch (err) {
      setError(`${done ? `Saved ${done}, then stopped: ` : ''}${errorText(err)}`)
    } finally {
      setBusy(false)
      res.reload()
    }
  }

  return (
    <div>
      <p className="muted small settings-hint">
        Monthly spend in rupees for each paid source. Website chat counts the website's running cost. Leave a cell blank
        if nothing is known; the dashboard then shows no cost for that source.
      </p>
      {notice && <Notice kind="good">{notice}</Notice>}
      {error && <Notice kind="error">{error}</Notice>}
      <div className="table-wrap table-scroll">
        <table className="table spend-table">
          <thead>
            <tr>
              <th>Source</th>
              {months.map((m) => (
                <th key={m}>{monthName(m)}</th>
              ))}
            </tr>
          </thead>
          <tbody>
            {SOURCES.map((src) => (
              <tr key={src}>
                <th scope="row">{sourceLabel[src]}</th>
                {months.map((m) => {
                  const k = `${src}|${m}`
                  return (
                    <td key={m}>
                      <input
                        className="input input-cell num"
                        inputMode="numeric"
                        aria-label={`${sourceLabel[src]}, ${monthName(m)}, rupees`}
                        placeholder={saved[k] != null ? undefined : '–'}
                        value={edits[k] ?? (saved[k] != null ? String(saved[k]) : '')}
                        onChange={(e) => setEdits({ ...edits, [k]: e.target.value.replace(/[,₹\s]/g, '') })}
                      />
                    </td>
                  )
                })}
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      <div className="actions" style={{ marginTop: 16 }}>
        <button className="btn btn-small" onClick={save} disabled={busy || changed.length === 0 || invalid}>
          {busy && <span className="spinner" aria-hidden="true" />}
          Save {changed.length || ''} change{changed.length === 1 ? '' : 's'}
        </button>
        {invalid && <span className="text-warn small">Use whole rupees, digits only.</span>}
        {changed.length > 0 && !invalid && (
          <span className="muted small">
            e.g. {rupees(Number(changed[0][1]))} for {sourceLabel[changed[0][0].split('|')[0] as Source]}
          </span>
        )}
      </div>
    </div>
  )
}
