import { useState } from 'react'
import { Link } from 'react-router-dom'
import type { ReminderEntry } from '../api/types'
import { Chip, EmptyState, ErrorState, Loading, PageHead } from '../components/ui'
import { dateTime } from '../lib/format'
import { reminderKindLabel, reminderReasonLabel, type Tone } from '../lib/labels'
import { useApi } from '../lib/useApi'
import './staff.css'

// Every visit reminder (1 day and 2 hours before) and what happened to it:
// waiting, sent, or skipped and why (the buyer opted out, the visit was
// cancelled ...). Messages are mocked: "sent" means written to the message
// log, ready for a WhatsApp/SMS provider.

const statusTone: Record<ReminderEntry['status'], Tone> = {
  queued: 'neutral',
  sent: 'good',
  skipped: 'quiet',
  failed: 'bad',
}
const statusLabel: Record<ReminderEntry['status'], string> = {
  queued: 'Waiting',
  sent: 'Sent',
  skipped: 'Skipped',
  failed: 'Failed',
}

export default function RemindersPage() {
  const [status, setStatus] = useState('')
  const res = useApi<{ reminders: ReminderEntry[] }>(`/staff/reminders${status ? `?status=${status}` : ''}`)
  const [shown, setShown] = useState<string | null>(null)
  const rows = res.data?.reminders ?? []

  return (
    <section>
      <PageHead
        title="Reminders"
        intro="Visit reminders from the last week and the coming week. Each includes the project's RERA number and how to opt out."
      >
        <select className="select" aria-label="Status" value={status} onChange={(e) => setStatus(e.target.value)}>
          <option value="">Any status</option>
          <option value="queued">Waiting</option>
          <option value="sent">Sent</option>
          <option value="skipped">Skipped</option>
          <option value="failed">Failed</option>
        </select>
      </PageHead>

      {res.error ? (
        <ErrorState message={res.error} onRetry={res.reload} />
      ) : res.loading && !res.data ? (
        <Loading label="Loading reminders" />
      ) : rows.length === 0 ? (
        <div className="table-wrap">
          <EmptyState title="No reminders">
            {status ? 'None with this status in these two weeks.' : 'Booking a visit queues two reminders for it.'}
          </EmptyState>
        </div>
      ) : (
        <div className="table-wrap">
          <table className="table table-cards">
            <thead>
              <tr>
                <th>Buyer</th>
                <th>Visit</th>
                <th>Reminder</th>
                <th>Status</th>
                <th>Message</th>
              </tr>
            </thead>
            <tbody>
              {rows.map((r) => (
                <tr key={r.id}>
                  <td>
                    <Link className="row-title" to={`/staff/leads/${r.leadId}`}>
                      {r.leadName}
                    </Link>
                    <div className="muted small num">{r.leadCode}</div>
                  </td>
                  <td data-label="Visit">
                    <span className="num">{dateTime(r.visitStartsAt)}</span>
                    <div className="muted small">
                      {r.projectName} · {r.executiveName}
                    </div>
                  </td>
                  <td data-label="Reminder">
                    {reminderKindLabel[r.kind]}
                    <div className="muted small num">
                      {r.sentAt ? `sent ${dateTime(r.sentAt)}` : `due ${dateTime(r.dueAt)}`}
                    </div>
                  </td>
                  <td data-label="Status">
                    <Chip tone={statusTone[r.status]}>{statusLabel[r.status]}</Chip>
                    {r.skipReason && (
                      <div className="muted small">{reminderReasonLabel[r.skipReason] ?? r.skipReason}</div>
                    )}
                  </td>
                  <td data-label="Message">
                    {r.body ? (
                      shown === r.id ? (
                        <>
                          <p className="reminder-body">{r.body}</p>
                          <button className="btn-link small" onClick={() => setShown(null)}>
                            Hide
                          </button>
                        </>
                      ) : (
                        <button className="btn-link small" onClick={() => setShown(r.id)}>
                          Show text
                        </button>
                      )
                    ) : (
                      <span className="muted small">Written when sent</span>
                    )}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </section>
  )
}
