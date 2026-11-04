import { useState, type FormEvent } from 'react'
import { api } from '../../api/client'
import type { Fact, Project } from '../../api/types'
import { EmptyState, ErrorState, Loading, Notice } from '../../components/ui'
import { dateTime } from '../../lib/format'
import { errorText, useApi } from '../../lib/useApi'
import { ConfirmButton, useProjects } from '../shared'

// The fact sheet: the only things the assistant may tell buyers. A change is
// used from the very next chat message. Anything not written here, the
// assistant does not guess: it says a person will confirm and hands off.
// Money figures in a reply must appear here, or the guardrail blocks the reply.

export default function FactsEditor() {
  const projects = useProjects()
  const [chosen, setChosen] = useState('')
  const projectId = chosen || projects[0]?.id || ''
  const res = useApi<{ project: Project; facts: Fact[] }>(
    projectId ? `/staff/projects/${encodeURIComponent(projectId)}/facts` : null,
  )
  const [editing, setEditing] = useState<string | null>(null) // a topic, or "new"
  const [notice, setNotice] = useState('')
  const [error, setError] = useState('')

  function saved(msg: string) {
    setEditing(null)
    setError('')
    setNotice(msg)
    res.reload()
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
        <button className="btn btn-small" onClick={() => setEditing('new')} disabled={editing === 'new'}>
          Add a fact
        </button>
      </div>
      <p className="muted small settings-hint">
        The assistant answers only from these facts, in English, Hindi or Gujarati. Leave out anything you do not want
        it to say: questions about it go to a salesperson. Never add discounts or final prices.
        {res.data && <> RERA number: <strong>{res.data.project.reraNumber}</strong>.</>}
      </p>
      {notice && <Notice kind="good">{notice}</Notice>}
      {error && <Notice kind="error">{error}</Notice>}

      {editing === 'new' && projectId && (
        <FactForm projectId={projectId} onCancel={() => setEditing(null)} onSaved={() => saved('Fact added. The assistant uses it from the next message.')} />
      )}

      {res.error ? (
        <ErrorState message={res.error} onRetry={res.reload} />
      ) : !res.data ? (
        <Loading label="Loading the fact sheet" />
      ) : res.data.facts.length === 0 ? (
        <div className="table-wrap">
          <EmptyState title="No facts yet">Until you add some, the assistant hands every question to a person.</EmptyState>
        </div>
      ) : (
        <ul className="facts">
          {res.data.facts.map((f) =>
            editing === f.topic ? (
              <li key={f.id}>
                <FactForm
                  projectId={projectId}
                  fact={f}
                  onCancel={() => setEditing(null)}
                  onSaved={() => saved('Fact saved. The assistant uses it from the next message.')}
                />
              </li>
            ) : (
              <li key={f.id} className="fact">
                <div>
                  <p className="fact-label">{f.label}</p>
                  <p className="fact-value">{f.value}</p>
                  <p className="muted small">Updated {dateTime(f.updatedAt)}</p>
                </div>
                <div className="actions">
                  <button className="btn btn-quiet btn-small" onClick={() => setEditing(f.topic)}>
                    Edit
                  </button>
                  <ConfirmButton
                    danger
                    question="Remove this fact? The assistant will hand these questions to a person."
                    confirmLabel="Remove"
                    onConfirm={async () => {
                      try {
                        await api(`/staff/projects/${projectId}/facts/${f.topic}`, { method: 'DELETE' })
                        saved(`"${f.label}" removed.`)
                      } catch (err) {
                        setNotice('')
                        setError(errorText(err))
                      }
                    }}
                  >
                    Remove
                  </ConfirmButton>
                </div>
              </li>
            ),
          )}
        </ul>
      )}
    </div>
  )
}

/** "Car parking (covered)" -> "car_parking_covered": the key the assistant looks facts up by. */
function topicFrom(label: string): string {
  let t = label
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, '_')
    .replace(/^_+|_+$/g, '')
    .slice(0, 40)
  if (!/^[a-z]/.test(t)) t = 'fact_' + t
  return t.replace(/_+$/, '').slice(0, 40)
}

function FactForm({
  projectId,
  fact,
  onSaved,
  onCancel,
}: {
  projectId: string
  fact?: Fact
  onSaved: () => void
  onCancel: () => void
}) {
  const [label, setLabel] = useState(fact?.label ?? '')
  const [value, setValue] = useState(fact?.value ?? '')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')

  async function submit(e: FormEvent) {
    e.preventDefault()
    const topic = fact?.topic ?? topicFrom(label)
    if (topic.length < 2) {
      setError('Give the fact a title.')
      return
    }
    setBusy(true)
    setError('')
    try {
      await api(`/staff/projects/${projectId}/facts/${topic}`, { method: 'PUT', body: { label, value } })
      onSaved()
    } catch (err) {
      setError(errorText(err))
    } finally {
      setBusy(false)
    }
  }

  return (
    <form className="panel fact-form" onSubmit={submit}>
      <div className="field">
        <label htmlFor={`ff-label-${fact?.id ?? 'new'}`}>Title</label>
        <input
          id={`ff-label-${fact?.id ?? 'new'}`}
          className="input"
          value={label}
          onChange={(e) => setLabel(e.target.value)}
          maxLength={80}
          placeholder="e.g. Possession date"
          required
        />
      </div>
      <div className="field">
        <label htmlFor={`ff-value-${fact?.id ?? 'new'}`}>What the assistant may say</label>
        <textarea
          id={`ff-value-${fact?.id ?? 'new'}`}
          className="textarea"
          value={value}
          onChange={(e) => setValue(e.target.value)}
          maxLength={1000}
          required
        />
        <span className="field-hint">{value.length} / 1,000 characters</span>
      </div>
      {error && <Notice kind="error">{error}</Notice>}
      <div className="actions">
        <button className="btn btn-small" type="submit" disabled={busy}>
          {busy && <span className="spinner" aria-hidden="true" />}
          Save
        </button>
        <button className="btn btn-quiet btn-small" type="button" onClick={onCancel} disabled={busy}>
          Cancel
        </button>
      </div>
    </form>
  )
}
