import { useState } from 'react'
import { Link } from 'react-router-dom'
import { api } from '../api/client'
import type { StaffVisit } from '../api/types'
import { useAuth } from '../auth/AuthContext'
import { Chip, EmptyState, ErrorState, Loading, Notice, PageHead } from '../components/ui'
import { addDays, isoDay, longDay, phone, time } from '../lib/format'
import { visitStatusLabel, visitStatusTone } from '../lib/labels'
import { errorText, useApi } from '../lib/useApi'
import { ConfirmButton } from './shared'
import './staff.css'

// The visit diary: who is coming, when, to which project. After a visit time
// has passed, the salesperson records whether the buyer came; that feeds the
// show-up rate on the dashboard.

type Range = 'coming' | 'past'

export default function VisitsPage() {
  const { user } = useAuth()
  const isHead = user?.role === 'head'
  const [range, setRange] = useState<Range>('coming')

  return (
    <section>
      <PageHead
        title="Visits"
        intro={isHead ? 'Every site visit across both projects.' : 'Your site visits. Record the outcome after each one.'}
      >
        <div className="segmented" role="group" aria-label="Which visits">
          <button aria-pressed={range === 'coming'} onClick={() => setRange('coming')}>
            Next two weeks
          </button>
          <button aria-pressed={range === 'past'} onClick={() => setRange('past')}>
            Last two weeks
          </button>
        </div>
      </PageHead>
      {/* Keyed by range, so switching never shows the other range's visits while loading. */}
      <Diary key={range} range={range} isHead={isHead} />
    </section>
  )
}

function Diary({ range, isHead }: { range: Range; isHead: boolean }) {
  const [error, setError] = useState('')
  const [notice, setNotice] = useState('')

  const today = isoDay(new Date())
  const path =
    range === 'coming'
      ? `/staff/visits?from=${today}&to=${addDays(today, 13)}`
      : `/staff/visits?from=${addDays(today, -14)}&to=${today}`
  const res = useApi<{ visits: StaffVisit[] }>(path)

  async function act(fn: () => Promise<unknown>, done: string) {
    setError('')
    setNotice('')
    try {
      await fn()
      setNotice(done)
      res.reload()
    } catch (err) {
      setError(errorText(err))
    }
  }

  let visits = res.data?.visits ?? []
  if (range === 'past') visits = [...visits].reverse() // most recent first when looking back
  const toMark = visits.filter((v) => v.canMark).length

  // Group by day for the diary layout.
  const days: { day: string; visits: StaffVisit[] }[] = []
  for (const v of visits) {
    const d = longDay(v.startsAt)
    const last = days[days.length - 1]
    if (last && last.day === d) last.visits.push(v)
    else days.push({ day: d, visits: [v] })
  }

  return (
    <>

      {error && <Notice kind="error">{error}</Notice>}
      {notice && <Notice kind="good">{notice}</Notice>}
      {toMark > 0 && (
        <Notice kind="warn">
          {toMark} visit{toMark === 1 ? ' is' : 's are'} waiting for an outcome: did the buyer come?
        </Notice>
      )}

      {res.error ? (
        <ErrorState message={res.error} onRetry={res.reload} />
      ) : res.loading && !res.data ? (
        <Loading label="Loading visits" />
      ) : days.length === 0 ? (
        <div className="table-wrap">
          <EmptyState title={range === 'coming' ? 'No visits booked yet' : 'No visits in the last two weeks'}>
            {range === 'coming'
              ? 'Visits booked by buyers in the chat, or by you from a lead page, appear here.'
              : 'Past visits and their outcomes appear here.'}
          </EmptyState>
        </div>
      ) : (
        <div className="diary">
          {days.map((d) => (
            <div key={d.day} className="diary-day">
              <h2 className="diary-date">{d.day}</h2>
              <ul className="diary-list">
                {d.visits.map((v) => (
                  <li key={v.id} className={`diary-item ${v.status === 'cancelled' ? 'is-cancelled' : ''}`}>
                    <span className="diary-time num">{time(v.startsAt)}</span>
                    <div className="diary-who">
                      <Link className="row-title" to={`/staff/leads/${v.leadId}`}>
                        {v.leadName}
                      </Link>
                      <span className="muted small num">
                        {v.leadCode} · <a href={`tel:${v.leadPhone}`}>{phone(v.leadPhone)}</a>
                      </span>
                      <span className="muted small">
                        {v.projectName}
                        {isHead && <> · with {v.executiveName}</>}
                      </span>
                    </div>
                    <div className="diary-actions">
                      <Chip tone={visitStatusTone[v.status]}>{visitStatusLabel[v.status]}</Chip>
                      {v.canMark && (
                        <div className="actions">
                          <button
                            className="btn btn-small"
                            onClick={() =>
                              act(
                                () => api(`/staff/visits/${v.id}/outcome`, { method: 'POST', body: { outcome: 'visited' } }),
                                `${v.leadName} marked as visited.`,
                              )
                            }
                          >
                            Came
                          </button>
                          <button
                            className="btn btn-quiet btn-small"
                            onClick={() =>
                              act(
                                () => api(`/staff/visits/${v.id}/outcome`, { method: 'POST', body: { outcome: 'no_show' } }),
                                `${v.leadName} marked as a no-show.`,
                              )
                            }
                          >
                            Did not come
                          </button>
                        </div>
                      )}
                      {v.status === 'booked' && !v.canMark && (
                        <ConfirmButton
                          danger
                          question="Cancel this visit?"
                          confirmLabel="Cancel visit"
                          onConfirm={() =>
                            act(() => api(`/staff/visits/${v.id}/cancel`, { method: 'POST' }), 'Visit cancelled. The slot is free again.')
                          }
                        >
                          Cancel
                        </ConfirmButton>
                      )}
                    </div>
                  </li>
                ))}
              </ul>
            </div>
          ))}
        </div>
      )}
    </>
  )
}
