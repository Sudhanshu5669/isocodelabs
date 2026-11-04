import { useState } from 'react'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { api } from '../api/client'
import type { LeadDetail } from '../api/types'
import { useAuth } from '../auth/AuthContext'
import { Chip, ErrorState, Loading, Notice } from '../components/ui'
import { dateTime, duration, phone, relative } from '../lib/format'
import { languageLabel, leadStatusLabel, leadStatusTone, sourceLabel, timelineLabel } from '../lib/labels'
import { errorText, useApi } from '../lib/useApi'
import ChatTranscript from './ChatTranscript'
import { budgetText, ConfirmButton, Detail, useTeam } from './shared'
import VisitPanel from './VisitPanel'
import './staff.css'

// One lead: who they are, what the assistant learned, the chat itself, the
// visit, and every enquiry they made. This is the handoff screen: the named
// salesperson opens it and has the whole story before calling.

export default function LeadDetailPage() {
  const { id = '' } = useParams()
  const { user } = useAuth()
  const isHead = user?.role === 'head'
  const navigate = useNavigate()
  const res = useApi<LeadDetail>(`/staff/leads/${encodeURIComponent(id)}`)
  const team = useTeam()

  const [error, setError] = useState('')
  const [notice, setNotice] = useState('')

  if (res.loading && !res.data) return <Loading label="Loading lead" />
  if (res.error || !res.data) return <ErrorState message={res.error || 'We could not find that lead.'} onRetry={res.reload} />
  const lead = res.data

  /** Runs one change, shows the outcome, then reloads the lead. Says whether it worked. */
  async function act(fn: () => Promise<unknown>, done: string): Promise<boolean> {
    setError('')
    setNotice('')
    try {
      await fn()
      setNotice(done)
      res.reload()
      return true
    } catch (err) {
      setError(errorText(err))
      window.scrollTo({ top: 0, behavior: 'smooth' })
      return false
    }
  }

  const budget = budgetText(lead.budgetMinLakh, lead.budgetMaxLakh)
  const executives = team.filter((u) => u.role === 'executive' && u.active)

  return (
    <section>
      <Link to="/staff" className="back-link">
        ← All leads
      </Link>

      <header className="lead-head">
        <div>
          <p className="muted small num">
            {lead.code} · came in {relative(lead.createdAt)} from {sourceLabel[lead.source]}
          </p>
          <h1>{lead.name}</h1>
        </div>
        <div className="row-flags">
          <Chip tone={leadStatusTone[lead.status]}>{leadStatusLabel[lead.status]}</Chip>
          {lead.optedOut && <Chip tone="bad">Opted out of messages</Chip>}
          {!lead.consent && !lead.optedOut && <Chip tone="warn">No consent recorded</Chip>}
        </div>
      </header>

      {error && <Notice kind="error">{error}</Notice>}
      {notice && <Notice kind="good">{notice}</Notice>}

      {lead.duplicateOf && (
        <div className="panel dup-banner">
          <div>
            <p className="eyebrow">Possible duplicate</p>
            <p>
              This may be the same person as{' '}
              <Link to={`/staff/leads/${lead.duplicateOf.id}`}>
                {lead.duplicateOf.name} ({lead.duplicateOf.code})
              </Link>
              : similar name, same project, different phone number.
            </p>
          </div>
          {isHead ? (
            <div className="actions">
              <ConfirmButton
                question={`Merge into ${lead.duplicateOf.code}? This lead's enquiries, chats and visits move there.`}
                confirmLabel="Merge"
                onConfirm={async () => {
                  setError('')
                  try {
                    const out = await api<{ leadId: string }>(`/staff/leads/${lead.id}/duplicate`, {
                      method: 'POST',
                      body: { action: 'merge' },
                    })
                    navigate(`/staff/leads/${out.leadId}`, { replace: true })
                  } catch (err) {
                    setError(errorText(err))
                  }
                }}
              >
                Merge into {lead.duplicateOf.code}
              </ConfirmButton>
              <button
                className="btn btn-quiet btn-small"
                onClick={() =>
                  act(
                    () => api(`/staff/leads/${lead.id}/duplicate`, { method: 'POST', body: { action: 'dismiss' } }),
                    'Marked as a different person.',
                  )
                }
              >
                Different people
              </button>
            </div>
          ) : (
            <p className="muted small">The sales head will merge these or keep them apart.</p>
          )}
        </div>
      )}

      <div className="lead-grid">
        <div className="lead-main">
          <div className="panel">
            <div className="panel-title">
              <h2>Conversation</h2>
              {lead.chats.length > 0 && <span className="muted small">Newest first</span>}
            </div>
            {lead.chats.length === 0 ? (
              <p className="muted">
                This buyer has not used the website chat. Their enquiries are listed under History.
              </p>
            ) : (
              <div className="transcripts">
                {lead.chats.map((c, i) => (
                  <ChatTranscript key={c.id} chat={c} open={i === 0} />
                ))}
              </div>
            )}
          </div>

          <div className="panel">
            <div className="panel-title">
              <h2>History</h2>
            </div>
            {lead.touches.length === 0 ? (
              <p className="muted">No enquiries recorded.</p>
            ) : (
              <ol className="timeline">
                {lead.touches.map((t) => (
                  <li key={t.id}>
                    <span className="timeline-when num">{dateTime(t.createdAt)}</span>
                    <span>
                      <strong>{sourceLabel[t.source as keyof typeof sourceLabel] ?? t.source}</strong>
                      {t.note && <span className="muted"> · {t.note}</span>}
                    </span>
                  </li>
                ))}
              </ol>
            )}
          </div>
        </div>

        <aside className="lead-side">
          <div className="panel">
            <div className="panel-title">
              <h2>Buyer</h2>
            </div>
            <dl className="details">
              <Detail label="Phone">
                <a href={`tel:${lead.phone}`} className="num">
                  {phone(lead.phone)}
                </a>
              </Detail>
              <Detail label="Project">{lead.projectName ?? <span className="muted">Not known</span>}</Detail>
              <Detail label="Looking for">{lead.wants ?? <span className="muted">Not known</span>}</Detail>
              <Detail label="Budget">{budget ?? <span className="muted">Not shared</span>}</Detail>
              <Detail label="Buying">
                {lead.timeline ? timelineLabel[lead.timeline] ?? lead.timeline : <span className="muted">Not shared</span>}
              </Detail>
              <Detail label="Language">{languageLabel[lead.language]}</Detail>
              <Detail label="First reply">
                {lead.firstResponseSeconds == null ? (
                  <span className="text-warn">Not yet</span>
                ) : (
                  <span className="num">{duration(lead.firstResponseSeconds)}</span>
                )}
              </Detail>
              <Detail label="Consent">
                {lead.optedOut
                  ? `Opted out ${lead.optedOutAt ? dateTime(lead.optedOutAt) : ''}`
                  : lead.consent
                    ? `Given ${lead.consentAt ? dateTime(lead.consentAt) : ''}`
                    : 'Not recorded'}
              </Detail>
              <Detail label="Salesperson">
                {isHead ? (
                  <select
                    className="select select-inline"
                    aria-label="Assign to"
                    value={lead.assignedTo ?? ''}
                    onChange={(e) =>
                      e.target.value &&
                      act(
                        () => api(`/staff/leads/${lead.id}/assign`, { method: 'POST', body: { executiveId: e.target.value } }),
                        'Lead assigned.',
                      )
                    }
                  >
                    {!lead.assignedTo && <option value="">Not assigned</option>}
                    {lead.assignedTo && !executives.some((x) => x.id === lead.assignedTo) && (
                      <option value={lead.assignedTo}>{lead.assigneeName}</option>
                    )}
                    {executives.map((x) => (
                      <option key={x.id} value={x.id}>
                        {x.name}
                      </option>
                    ))}
                  </select>
                ) : (
                  (lead.assigneeName ?? 'Not assigned')
                )}
              </Detail>
            </dl>
          </div>

          <VisitPanel lead={lead} act={act} />

          <div className="panel">
            <div className="panel-title">
              <h2>Lead</h2>
            </div>
            <div className="actions">
              {lead.status === 'lost' ? (
                <button
                  className="btn btn-quiet btn-small"
                  onClick={() =>
                    act(
                      () => api(`/staff/leads/${lead.id}/status`, { method: 'POST', body: { status: 'engaged' } }),
                      'Lead reopened.',
                    )
                  }
                >
                  Reopen lead
                </button>
              ) : (
                lead.status !== 'visit_booked' && (
                  <ConfirmButton
                    question="Mark this lead as lost?"
                    confirmLabel="Mark lost"
                    onConfirm={() =>
                      act(
                        () => api(`/staff/leads/${lead.id}/status`, { method: 'POST', body: { status: 'lost' } }),
                        'Marked as lost.',
                      )
                    }
                  >
                    Mark as lost
                  </ConfirmButton>
                )
              )}
              {!lead.optedOut && (
                <ConfirmButton
                  danger
                  question="Record that the buyer asked not to be contacted? Pending reminders stop."
                  confirmLabel="Record opt-out"
                  onConfirm={() =>
                    act(() => api(`/staff/leads/${lead.id}/opt-out`, { method: 'POST' }), 'Opt-out recorded. No more messages will be sent.')
                  }
                >
                  Buyer opted out
                </ConfirmButton>
              )}
            </div>
            {lead.status === 'visit_booked' && (
              <p className="muted small" style={{ marginTop: 12 }}>
                To close this lead, cancel the booked visit first.
              </p>
            )}
          </div>
        </aside>
      </div>
    </section>
  )
}
