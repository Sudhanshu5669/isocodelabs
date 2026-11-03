import { useState } from 'react'
import { Link, useParams, useSearchParams } from 'react-router-dom'
import { api } from '../api/client'
import type { FreeSlot, Lang, VisitDetail } from '../api/types'
import SlotPicker from '../components/SlotPicker'
import VisitPass from '../components/VisitPass'
import { ErrorState, Loading } from '../components/ui'
import { addDays, isoDay } from '../lib/format'
import { errorText, useApi } from '../lib/useApi'
import { languageNames, shortDay, words } from './words'
import './visit.css'

// The buyer's own visit page, reached through the unguessable code in their
// link. No sign-in: whoever holds the link can see, move or cancel this one
// visit, and nothing else.

export default function VisitPage() {
  const { code = '' } = useParams()
  const [params, setParams] = useSearchParams()
  const lang = (['en', 'hi', 'gu'].includes(params.get('lang') ?? '') ? params.get('lang') : 'en') as Lang
  const w = words[lang]

  const visit = useApi<VisitDetail>(`/visits/${encodeURIComponent(code)}`)
  const [current, setCurrent] = useState<VisitDetail | null>(null)
  const v = current ?? visit.data

  const [panel, setPanel] = useState<'none' | 'reschedule' | 'cancel'>('none')
  const [busy, setBusy] = useState('')
  const [notice, setNotice] = useState('')
  const [error, setError] = useState('')

  const today = isoDay(new Date())
  const slots = useApi<{ slots: FreeSlot[] }>(
    panel === 'reschedule' && v ? `/slots?project=${v.projectId}&from=${today}&to=${addDays(today, 13)}` : null,
  )

  async function act(path: string, body: unknown, done: string) {
    setError('')
    setNotice('')
    setBusy(path)
    try {
      setCurrent(await api<VisitDetail>(`/visits/${code}/${path}`, { method: 'POST', body }))
      setNotice(done)
      setPanel('none')
    } catch (err) {
      setError(errorText(err))
      if (path === 'reschedule') slots.reload() // the time may just have been taken
    } finally {
      setBusy('')
    }
  }

  if (visit.error) {
    return (
      <main className="pub-main">
        <ErrorState message={visit.error} onRetry={visit.reload} />
      </main>
    )
  }
  if (!v) {
    return (
      <main className="pub-main">
        <Loading />
      </main>
    )
  }

  return (
    <main className="pub-main visit" lang={lang}>
      <div className="segmented" role="group" aria-label="Language">
        {(['en', 'hi', 'gu'] as Lang[]).map((l) => (
          <button key={l} type="button" lang={l} aria-pressed={lang === l} onClick={() => setParams({ lang: l }, { replace: true })}>
            {languageNames[l]}
          </button>
        ))}
      </div>

      <p className="eyebrow" style={{ marginTop: 28 }}>
        {w.visitTitle}
      </p>
      <h1>{w.visitHello(v.leadFirstName)}</h1>

      <div className="visit-pass">
        <VisitPass visit={v} words={w.pass} />
      </div>

      <div aria-live="polite">
        {notice && <p className="alert alert-good">{notice}</p>}
        {error && <p className="alert alert-error">{error}</p>}
      </div>

      {v.canChange ? (
        <div className="visit-actions">
          {panel === 'none' && (
            <div className="actions">
              <button className="btn" onClick={() => setPanel('reschedule')}>
                {w.reschedule}
              </button>
              <button className="btn btn-quiet" onClick={() => setPanel('cancel')}>
                {w.cancelVisit}
              </button>
            </div>
          )}

          {panel === 'reschedule' && (
            <section className="panel">
              <p>{w.pickNewTime}</p>
              <div style={{ marginTop: 16 }}>
                {slots.loading ? (
                  <Loading />
                ) : (slots.data?.slots.filter((s) => s.id !== v.slotId).length ?? 0) === 0 ? (
                  <p className="muted">{w.noFreeTimes}</p>
                ) : (
                  <SlotPicker
                    slots={slots.data!.slots.filter((s) => s.id !== v.slotId)}
                    onPick={(s) => act('reschedule', { slotId: s.id }, w.rescheduled)}
                    busyId={busy === 'reschedule' ? 'x' : undefined}
                    formatDay={shortDay(lang)}
                    withLabel={w.withLabel}
                  />
                )}
              </div>
              <button className="btn btn-quiet btn-small" style={{ marginTop: 16 }} onClick={() => setPanel('none')}>
                {w.close}
              </button>
            </section>
          )}

          {panel === 'cancel' && (
            <section className="panel" role="alertdialog" aria-label={w.cancelVisit}>
              <p>{w.cancelConfirm}</p>
              <div className="actions" style={{ marginTop: 16 }}>
                <button className="btn btn-danger" disabled={!!busy} onClick={() => act('cancel', undefined, w.cancelled)}>
                  {busy === 'cancel' && <span className="spinner" aria-hidden="true" />} {w.cancelVisit}
                </button>
                <button className="btn btn-quiet" onClick={() => setPanel('none')}>
                  {w.keepVisit}
                </button>
              </div>
            </section>
          )}
        </div>
      ) : (
        v.status === 'booked' && <p className="muted">{w.visitClosed}</p>
      )}

      <section className="visit-messages">
        {v.optedOut ? (
          <p className="muted">{w.stopped}</p>
        ) : (
          <>
            <p className="muted">{w.stopExplain}</p>
            <button className="btn-link" disabled={!!busy} onClick={() => act('opt-out', undefined, w.stopped)}>
              {w.stopMessages}
            </button>
          </>
        )}
      </section>

      <p style={{ marginTop: 32 }}>
        <Link to="/chat">{w.talkToPerson}</Link>
      </p>
    </main>
  )
}
