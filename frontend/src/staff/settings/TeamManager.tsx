import { useState, type FormEvent } from 'react'
import { api } from '../../api/client'
import type { DeactivateResult, StaffUser } from '../../api/types'
import { useAuth } from '../../auth/AuthContext'
import { Chip, ErrorState, Loading, Notice } from '../../components/ui'
import { errorText, useApi } from '../../lib/useApi'
import { ConfirmButton } from '../shared'

// The team. There is no self sign-up: the head adds each person with a first
// password and passes it on privately. Switching someone off signs them out at
// once and withdraws their open slots; their leads and visits stay for the head
// to hand to someone else.

export default function TeamManager() {
  const { user: me } = useAuth()
  const res = useApi<{ users: StaffUser[] }>('/staff/users')
  const [adding, setAdding] = useState(false)
  const [resetFor, setResetFor] = useState<string | null>(null)
  const [notice, setNotice] = useState('')
  const [error, setError] = useState('')

  async function setActive(u: StaffUser, active: boolean) {
    setError('')
    setNotice('')
    try {
      const r = await api<DeactivateResult>(`/staff/users/${u.id}/active`, { method: 'POST', body: { active } })
      if (active) {
        setNotice(`${u.name} can sign in again.`)
      } else {
        const todo = [
          r.bookedVisits && `${r.bookedVisits} upcoming visit${r.bookedVisits === 1 ? '' : 's'}`,
          r.assignedLeads && `${r.assignedLeads} open lead${r.assignedLeads === 1 ? '' : 's'}`,
        ].filter(Boolean)
        setNotice(
          `${u.name} is switched off and signed out. ${r.cancelledOpenSlots} open slot${r.cancelledOpenSlots === 1 ? ' was' : 's were'} withdrawn.` +
            (todo.length ? ` Still to hand to someone else: ${todo.join(' and ')}.` : ''),
        )
      }
      res.reload()
    } catch (err) {
      setError(errorText(err))
    }
  }

  return (
    <div>
      <div className="settings-bar">
        <p className="muted small">People who can sign in to the staff area.</p>
        <button className="btn btn-small" onClick={() => setAdding((a) => !a)} aria-expanded={adding}>
          {adding ? 'Close' : 'Add a person'}
        </button>
      </div>
      {notice && <Notice kind="good">{notice}</Notice>}
      {error && <Notice kind="error">{error}</Notice>}
      {adding && (
        <AddUser
          onDone={(name) => {
            setAdding(false)
            setNotice(`${name} added. Give them their first password in person or by phone, not by email.`)
            res.reload()
          }}
        />
      )}

      {res.error ? (
        <ErrorState message={res.error} onRetry={res.reload} />
      ) : !res.data ? (
        <Loading label="Loading the team" />
      ) : (
        <div className="table-wrap">
          <table className="table table-cards">
            <thead>
              <tr>
                <th>Name</th>
                <th>Role</th>
                <th>Open leads</th>
                <th>Upcoming visits</th>
                <th>
                  <span className="visually-hidden">Actions</span>
                </th>
              </tr>
            </thead>
            <tbody>
              {res.data.users.map((u) => (
                <tr key={u.id} className={u.active ? '' : 'is-off'}>
                  <td>
                    <strong>{u.name}</strong>
                    {u.id === me?.id && <span className="muted small"> (you)</span>}
                    <div className="muted small">{u.email}</div>
                  </td>
                  <td data-label="Role">
                    {u.role === 'head' ? 'Sales head' : 'Executive'}{' '}
                    {!u.active && <Chip tone="quiet">Switched off</Chip>}
                  </td>
                  <td data-label="Open leads" className="num">
                    {u.openLeads}
                  </td>
                  <td data-label="Upcoming visits" className="num">
                    {u.upcomingVisits}
                  </td>
                  <td>
                    {resetFor === u.id ? (
                      <ResetPassword
                        user={u}
                        onDone={(msg) => {
                          setResetFor(null)
                          if (msg) setNotice(msg)
                        }}
                      />
                    ) : (
                      <div className="actions">
                        <button className="btn btn-quiet btn-small" onClick={() => setResetFor(u.id)}>
                          New password
                        </button>
                        {u.id !== me?.id &&
                          (u.active ? (
                            <ConfirmButton
                              danger
                              question={`Switch off ${u.name}? They are signed out at once.`}
                              confirmLabel="Switch off"
                              onConfirm={() => setActive(u, false)}
                            >
                              Switch off
                            </ConfirmButton>
                          ) : (
                            <button className="btn btn-quiet btn-small" onClick={() => setActive(u, true)}>
                              Switch on
                            </button>
                          ))}
                      </div>
                    )}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  )
}

function AddUser({ onDone }: { onDone: (name: string) => void }) {
  const [form, setForm] = useState({ name: '', email: '', role: 'executive', password: '' })
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')

  async function submit(e: FormEvent) {
    e.preventDefault()
    setBusy(true)
    setError('')
    try {
      await api('/staff/users', { method: 'POST', body: form })
      onDone(form.name)
    } catch (err) {
      setError(errorText(err))
    } finally {
      setBusy(false)
    }
  }

  return (
    <form className="panel add-user" onSubmit={submit}>
      <div className="form-grid">
        <div className="field">
          <label htmlFor="au-name">Name</label>
          <input id="au-name" className="input" value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} maxLength={80} required />
        </div>
        <div className="field">
          <label htmlFor="au-email">Work email</label>
          <input id="au-email" className="input" type="email" value={form.email} onChange={(e) => setForm({ ...form, email: e.target.value })} required />
        </div>
        <div className="field">
          <label htmlFor="au-role">Role</label>
          <select id="au-role" className="select" value={form.role} onChange={(e) => setForm({ ...form, role: e.target.value })}>
            <option value="executive">Executive</option>
            <option value="head">Sales head</option>
          </select>
        </div>
        <div className="field">
          <label htmlFor="au-pw">First password</label>
          <input
            id="au-pw"
            className="input"
            type="text"
            autoComplete="new-password"
            value={form.password}
            onChange={(e) => setForm({ ...form, password: e.target.value })}
            minLength={10}
            maxLength={72}
            required
          />
          <span className="field-hint">At least 10 characters.</span>
        </div>
      </div>
      {error && <Notice kind="error">{error}</Notice>}
      <div className="actions">
        <button className="btn btn-small" type="submit" disabled={busy}>
          {busy && <span className="spinner" aria-hidden="true" />}
          Add person
        </button>
      </div>
    </form>
  )
}

function ResetPassword({ user, onDone }: { user: StaffUser; onDone: (msg?: string) => void }) {
  const [pw, setPw] = useState('')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')

  async function submit(e: FormEvent) {
    e.preventDefault()
    setBusy(true)
    setError('')
    try {
      await api(`/staff/users/${user.id}/password`, { method: 'POST', body: { password: pw } })
      onDone(`New password set for ${user.name}. They are signed out everywhere and must use it next time.`)
    } catch (err) {
      setError(errorText(err))
      setBusy(false)
    }
  }

  return (
    <form className="reset-pw" onSubmit={submit}>
      <input
        className="input"
        type="text"
        autoComplete="new-password"
        aria-label={`New password for ${user.name}`}
        placeholder="New password (10+ characters)"
        value={pw}
        onChange={(e) => setPw(e.target.value)}
        minLength={10}
        maxLength={72}
        required
      />
      <div className="actions">
        <button className="btn btn-small" type="submit" disabled={busy}>
          Set
        </button>
        <button className="btn btn-quiet btn-small" type="button" onClick={() => onDone()} disabled={busy}>
          Cancel
        </button>
      </div>
      {error && <p className="text-danger small">{error}</p>}
    </form>
  )
}
