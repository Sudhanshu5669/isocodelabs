import { useState } from 'react'
import { Link } from 'react-router-dom'
import { api } from '../api/client'
import type { FreeSlot, LeadDetail } from '../api/types'
import { Chip, ErrorState, Loading } from '../components/ui'
import SlotPicker from '../components/SlotPicker'
import { addDays, day, isoDay, longDay, time } from '../lib/format'
import { visitStatusLabel, visitStatusTone } from '../lib/labels'
import { useApi } from '../lib/useApi'
import { ConfirmButton, useProjects } from './shared'

// The lead's site visit: see it, record whether the buyer came, cancel it, or
// book one for them (e.g. after a phone call). The times offered come from the
// server's list of free slots, and the server re-checks the slot is still free
// when booking, so two people can never get the same slot.

type Act = (fn: () => Promise<unknown>, done: string) => Promise<boolean>

export default function VisitPanel({ lead, act }: { lead: LeadDetail; act: Act }) {
  const v = lead.visit
  const [booking, setBooking] = useState(false)

  return (
    <div className="panel">
      <div className="panel-title">
        <h2>Site visit</h2>
        {v && <Chip tone={visitStatusTone[v.status]}>{visitStatusLabel[v.status]}</Chip>}
      </div>

      {v ? (
        <div className="visit-box">
          <p className="visit-when">
            {longDay(v.startsAt)}, <span className="num">{time(v.startsAt)}</span>
          </p>
          <p className="muted small">
            {v.projectName} · with {v.executiveName} ·{' '}
            <Link to={`/visit/${v.code}`} target="_blank" rel="noreferrer">
              buyer's page ↗
            </Link>
          </p>
          {v.status === 'booked' && (
            <div className="actions" style={{ marginTop: 14 }}>
              {new Date(v.startsAt).getTime() <= Date.now() ? (
                <>
                  <button
                    className="btn btn-small"
                    onClick={() =>
                      act(
                        () => api(`/staff/visits/${v.id}/outcome`, { method: 'POST', body: { outcome: 'visited' } }),
                        'Marked as visited.',
                      )
                    }
                  >
                    Came for the visit
                  </button>
                  <button
                    className="btn btn-quiet btn-small"
                    onClick={() =>
                      act(
                        () => api(`/staff/visits/${v.id}/outcome`, { method: 'POST', body: { outcome: 'no_show' } }),
                        'Marked as a no-show.',
                      )
                    }
                  >
                    Did not come
                  </button>
                </>
              ) : (
                <ConfirmButton
                  danger
                  question="Cancel this visit? The slot opens for other buyers."
                  confirmLabel="Cancel visit"
                  onConfirm={() => act(() => api(`/staff/visits/${v.id}/cancel`, { method: 'POST' }), 'Visit cancelled.')}
                >
                  Cancel visit
                </ConfirmButton>
              )}
            </div>
          )}
        </div>
      ) : (
        <p className="muted">No visit booked yet.</p>
      )}

      {v?.status !== 'booked' &&
        (lead.status === 'lost' ? (
          <p className="muted small" style={{ marginTop: 12 }}>
            Reopen the lead to book a visit.
          </p>
        ) : booking ? (
          <BookVisit
            lead={lead}
            onClose={() => setBooking(false)}
            onBook={(slot) =>
              act(
                () => api('/staff/visits', { method: 'POST', body: { leadId: lead.id, slotId: slot.id } }),
                `Visit booked for ${day(slot.startsAt)} at ${time(slot.startsAt)}. Reminders go out 1 day and 2 hours before.`,
              ).then((ok) => {
                if (ok) setBooking(false)
              })
            }
          />
        ) : (
          <button className="btn btn-small" style={{ marginTop: 14 }} onClick={() => setBooking(true)}>
            {v ? 'Book another visit' : 'Book a visit'}
          </button>
        ))}
    </div>
  )
}

function BookVisit({
  lead,
  onBook,
  onClose,
}: {
  lead: LeadDetail
  onBook: (slot: FreeSlot) => Promise<void>
  onClose: () => void
}) {
  const projects = useProjects()
  const [projectId, setProjectId] = useState(lead.projectId ?? '')
  const [busyId, setBusyId] = useState('')
  const project = projectId || projects[0]?.id || ''

  const today = isoDay(new Date())
  const slots = useApi<{ slots: FreeSlot[] }>(
    project ? `/slots?project=${encodeURIComponent(project)}&from=${today}&to=${addDays(today, 13)}` : null,
  )

  return (
    <div className="book-visit">
      <div className="field">
        <label htmlFor="bv-project">Project</label>
        <select id="bv-project" className="select" value={project} onChange={(e) => setProjectId(e.target.value)}>
          {projects.map((p) => (
            <option key={p.id} value={p.id}>
              {p.name}
            </option>
          ))}
        </select>
      </div>
      <p className="small muted">Free times in the next two weeks</p>
      {slots.error ? (
        <ErrorState message={slots.error} onRetry={slots.reload} />
      ) : slots.loading || !slots.data ? (
        <Loading label="Loading free times" />
      ) : slots.data.slots.length === 0 ? (
        <p className="muted small">No free times in the next two weeks. The sales head can add slots in Settings.</p>
      ) : (
        <SlotPicker
          slots={slots.data.slots}
          busyId={busyId}
          onPick={async (s) => {
            setBusyId(s.id)
            try {
              await onBook(s)
            } finally {
              setBusyId('')
              slots.reload() // someone may have taken a slot meanwhile
            }
          }}
        />
      )}
      <button className="btn-link small" onClick={onClose}>
        Close
      </button>
    </div>
  )
}
