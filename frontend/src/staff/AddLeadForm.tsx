import { useState, type FormEvent } from 'react'
import { api } from '../api/client'
import type { Project } from '../api/types'
import { Notice } from '../components/ui'
import { errorText } from '../lib/useApi'

// Logging an enquiry that came in by phone, at a hoarding or from a walk-in.
// The server checks the phone number against every lead, so a repeat enquiry
// is added to the existing lead instead of making a second one.

type IngestResponse = {
  outcome: 'created' | 'merged'
  lead?: { id: string; code: string }
  duplicateOfCode?: string
}

export default function AddLeadForm({
  projects,
  onDone,
}: {
  projects: Project[]
  /** Called after saving; with the lead's id when this user may open it. */
  onDone: (leadId?: string) => void
}) {
  const [form, setForm] = useState({
    name: '',
    phone: '',
    source: 'hoarding_call',
    projectId: '',
    wants: '',
    language: 'en',
    note: '',
    consent: false,
  })
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const [notice, setNotice] = useState('')

  const set = (k: keyof typeof form) => (e: { target: { value: string } }) => setForm({ ...form, [k]: e.target.value })

  async function submit(e: FormEvent) {
    e.preventDefault()
    setError('')
    setNotice('')
    if (!form.consent) {
      setError('Tick the consent box: we may only contact people who agreed to it.')
      return
    }
    setBusy(true)
    try {
      const res = await api<IngestResponse>('/staff/leads', { method: 'POST', body: form })
      if (res.outcome === 'created') {
        onDone(res.lead?.id)
        return
      }
      // The phone number already belonged to a lead: the enquiry was added to it.
      if (res.lead) {
        onDone(res.lead.id)
      } else {
        setNotice(
          'This number already belongs to a lead assigned to another executive. The enquiry was added to that lead, and they will see it.',
        )
      }
    } catch (err) {
      setError(errorText(err))
    } finally {
      setBusy(false)
    }
  }

  return (
    <form className="panel add-lead" onSubmit={submit} noValidate>
      <div className="panel-title">
        <h2>Add a lead</h2>
        <span className="muted small">If the number is already known, this enquiry joins that lead.</span>
      </div>
      <div className="form-grid">
        <div className="field">
          <label htmlFor="al-name">Name</label>
          <input id="al-name" className="input" value={form.name} onChange={set('name')} maxLength={80} required />
        </div>
        <div className="field">
          <label htmlFor="al-phone">Mobile number</label>
          <input
            id="al-phone"
            className="input"
            type="tel"
            inputMode="tel"
            placeholder="98765 43210"
            value={form.phone}
            onChange={set('phone')}
            maxLength={20}
            required
          />
        </div>
        <div className="field">
          <label htmlFor="al-source">Came from</label>
          <select id="al-source" className="select" value={form.source} onChange={set('source')}>
            <option value="hoarding_call">Hoarding call</option>
            <option value="property_portal">Property portal</option>
            <option value="social_ad">Social media ad</option>
            <option value="manual">Walk-in or referral</option>
          </select>
        </div>
        <div className="field">
          <label htmlFor="al-project">Project</label>
          <select id="al-project" className="select" value={form.projectId} onChange={set('projectId')}>
            <option value="">Not sure yet</option>
            {projects.map((p) => (
              <option key={p.id} value={p.id}>
                {p.name}
              </option>
            ))}
          </select>
        </div>
        <div className="field">
          <label htmlFor="al-wants">Looking for</label>
          <select id="al-wants" className="select" value={form.wants} onChange={set('wants')}>
            <option value="">Not sure yet</option>
            <option value="2 BHK">2 BHK</option>
            <option value="3 BHK">3 BHK</option>
          </select>
        </div>
        <div className="field">
          <label htmlFor="al-lang">Prefers</label>
          <select id="al-lang" className="select" value={form.language} onChange={set('language')}>
            <option value="en">English</option>
            <option value="hi">Hindi</option>
            <option value="gu">Gujarati</option>
          </select>
        </div>
      </div>
      <div className="field" style={{ marginTop: 16 }}>
        <label htmlFor="al-note">Note (optional)</label>
        <input
          id="al-note"
          className="input"
          value={form.note}
          onChange={set('note')}
          maxLength={200}
          placeholder="e.g. Called from the SG Highway hoarding, wants east facing"
        />
      </div>
      <label className="check" style={{ marginTop: 16 }}>
        <input
          type="checkbox"
          checked={form.consent}
          onChange={(e) => setForm({ ...form, consent: e.target.checked })}
        />
        <span>The buyer agreed to be contacted by Company A about this enquiry (needed under the DPDP Act).</span>
      </label>
      {error && (
        <div style={{ marginTop: 16 }}>
          <Notice kind="error">{error}</Notice>
        </div>
      )}
      {notice && (
        <div style={{ marginTop: 16 }}>
          <Notice kind="warn">{notice}</Notice>
        </div>
      )}
      <div className="actions" style={{ marginTop: 20 }}>
        <button className="btn" type="submit" disabled={busy}>
          {busy && <span className="spinner" aria-hidden="true" />}
          Save lead
        </button>
        <button className="btn btn-quiet" type="button" onClick={() => onDone()} disabled={busy}>
          Cancel
        </button>
      </div>
    </form>
  )
}
