import { useState, type FormEvent } from 'react'
import { Link } from 'react-router-dom'
import { api } from '../../api/client'
import type { StaffSlot, StaffUser } from '../../api/types'
import { Chip, EmptyState, ErrorState, Loading, Notice } from '../../components/ui'
import { addDays, isoDay, longDay, time } from '../../lib/format'
import { errorText, useApi } from '../../lib/useApi'
import { ConfirmButton, useProjects } from '../shared'

// Visit slots: the one-hour times buyers can book. The head opens slots for an
// executive at a project; buyers (in the chat) and staff (on a lead) can only
// book an open slot. The database refuses two live visits in one slot and an
// executive in two places at once, so double booking cannot happen.

const HOURS = [9, 10, 11, 12, 13, 14, 15, 16, 17, 18]

export default function SlotsManager() {
  const projects = useProjects()
  const [chosen, setChosen] = useState('')
  const projectId = chosen || projects[0]?.id || ''
  const today = isoDay(new Date())
  const res = useApi<{ slots: StaffSlot[] }>(
    projectId ? `/staff/slots?project=${projectId}&from=${today}&to=${addDays(today, 13)}` : null,
  )
  const [adding, setAdding] = useState(false)
  const [notice, setNotice] = useState('')
  const [error, setError] = useState('')

  const live = (res.data?.slots ?? []).filter((s) => s.status !== 'cancelled')
  const days: { day: string; slots: StaffSlot[] }[] = []
  for (const s of live) {
    const d = longDay(s.startsAt)
    const last = days[days.length - 1]
    if (last && last.day === d) last.slots.push(s)
    else days.push({ day: d, slots: [s] })
  }

  return (
    <div>
      <div className="settings-bar">
        <div className="segmented" role="group" aria-label="Project">
          {projects.map((p) => (
            <button key={p.id} aria-pressed={projectId === p.id} onClick={() => setChosen(p.id)}>
              {p.name}
            </button>
          ))}
        </div>
        <button className="btn btn-small" onClick={() => setAdding((a) => !a)} aria-expanded={adding}>
          {adding ? 'Close' : 'Open new slots'}
        </button>
      </div>
      <p className="muted small settings-hint">
        The next two weeks. Buyers see open slots at least 2 hours ahead. A booked slot cannot be removed until its
        visit is cancelled.
      </p>
      {notice && <Notice kind="good">{notice}</Notice>}
      {error && <Notice kind="error">{error}</Notice>}

      {adding && projectId && (
        <AddSlots
          projectId={projectId}
          onDone={(msg, failed) => {
            setNotice(msg)
            setError(failed)
            if (!failed) setAdding(false)
            res.reload()
          }}
        />
      )}

      {res.error ? (
        <ErrorState message={res.error} onRetry={res.reload} />
      ) : !res.data ? (
        <Loading label="Loading slots" />
      ) : days.length === 0 ? (
        <div className="table-wrap">
          <EmptyState title="No slots in the next two weeks">Open some, or buyers will have nothing to book.</EmptyState>
        </div>
      ) : (
        <div className="diary">
          {days.map((d) => (
            <div key={d.day} className="diary-day">
              <h2 className="diary-date">{d.day}</h2>
              <ul className="diary-list">
                {d.slots.map((s) => (
                  <li key={s.id} className="diary-item">
                    <span className="diary-time num">
                      {time(s.startsAt)}–{time(s.endsAt)}
                    </span>
                    <div className="diary-who">
                      <strong>{s.executiveName}</strong>
                      {s.status === 'booked' && s.leadName && <span className="muted small">Booked by {s.leadName}</span>}
                    </div>
                    <div className="diary-actions">
                      <Chip tone={s.status === 'open' ? 'neutral' : 'good'}>{s.status === 'open' ? 'Open' : 'Booked'}</Chip>
                      {s.status === 'open' ? (
                        <ConfirmButton
                          question="Remove this slot?"
                          confirmLabel="Remove"
                          onConfirm={async () => {
                            setError('')
                            try {
                              await api(`/staff/slots/${s.id}`, { method: 'DELETE' })
                              setNotice('Slot removed.')
                              res.reload()
                            } catch (err) {
                              setError(errorText(err))
                            }
                          }}
                        >
                          Remove
                        </ConfirmButton>
                      ) : (
                        <Link className="small" to="/staff/visits">
                          See visit
                        </Link>
                      )}
                    </div>
                  </li>
                ))}
              </ul>
            </div>
          ))}
        </div>
      )}
    </div>
  )
}

function AddSlots({ projectId, onDone }: { projectId: string; onDone: (msg: string, failed: string) => void }) {
  const teamRes = useApi<{ users: StaffUser[] }>('/staff/users')
  const team = (teamRes.data?.users ?? []).filter((u) => u.role === 'executive' && u.active)
  const [exec, setExec] = useState('')
  const [date, setDate] = useState(addDays(isoDay(new Date()), 1))
  const [hours, setHours] = useState<number[]>([])
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const executiveId = exec || team[0]?.id || ''

  async function submit(e: FormEvent) {
    e.preventDefault()
    setError('')
    if (!executiveId || hours.length === 0) {
      setError('Choose an executive and at least one time.')
      return
    }
    setBusy(true)
    const failures: string[] = []
    let made = 0
    // One request per slot: each is checked on its own (e.g. an overlap
    // refuses that hour only).
    for (const h of [...hours].sort((a, b) => a - b)) {
      const hh = String(h).padStart(2, '0')
      const end = String(h + 1).padStart(2, '0')
      try {
        await api('/staff/slots', {
          method: 'POST',
          body: {
            projectId,
            executiveId,
            startsAt: `${date}T${hh}:00:00+05:30`,
            endsAt: `${date}T${end}:00:00+05:30`,
          },
        })
        made++
      } catch (err) {
        failures.push(`${hourLabel(h)}: ${errorText(err)}`)
      }
    }
    setBusy(false)
    onDone(made ? `${made} slot${made === 1 ? '' : 's'} opened.` : '', failures.join(' '))
  }

  if (teamRes.error) return <ErrorState message={teamRes.error} onRetry={teamRes.reload} />
  if (!teamRes.data) return <Loading label="Loading the team" />
  if (team.length === 0) {
    return <Notice kind="warn">There are no active executives. Add one under Team first.</Notice>
  }

  return (
    <form className="panel add-slots" onSubmit={submit}>
      <div className="form-grid">
        <div className="field">
          <label htmlFor="as-exec">Executive</label>
          <select id="as-exec" className="select" value={executiveId} onChange={(e) => setExec(e.target.value)}>
            {team.map((u) => (
              <option key={u.id} value={u.id}>
                {u.name}
              </option>
            ))}
          </select>
        </div>
        <div className="field">
          <label htmlFor="as-date">Day</label>
          <input
            id="as-date"
            className="input"
            type="date"
            min={isoDay(new Date())}
            value={date}
            onChange={(e) => setDate(e.target.value)}
            required
          />
        </div>
      </div>
      <fieldset className="hour-picks">
        <legend>Times (one hour each)</legend>
        {HOURS.map((h) => (
          <label key={h} className={`hour-pick ${hours.includes(h) ? 'is-on' : ''}`}>
            <input
              type="checkbox"
              checked={hours.includes(h)}
              onChange={(e) => setHours(e.target.checked ? [...hours, h] : hours.filter((x) => x !== h))}
            />
            {hourLabel(h)}
          </label>
        ))}
      </fieldset>
      {error && <Notice kind="error">{error}</Notice>}
      <div className="actions">
        <button className="btn btn-small" type="submit" disabled={busy}>
          {busy && <span className="spinner" aria-hidden="true" />}
          Open {hours.length || ''} slot{hours.length === 1 ? '' : 's'}
        </button>
      </div>
    </form>
  )
}

const hourLabel = (h: number) => `${h % 12 || 12}:00 ${h < 12 ? 'am' : 'pm'}`
