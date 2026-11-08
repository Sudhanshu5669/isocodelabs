import { useEffect, useLayoutEffect, useMemo, useRef, useState, type FormEvent, type KeyboardEvent, type ReactNode } from 'react'
import { Link, useSearchParams } from 'react-router-dom'
import { api, ApiError } from '../api/client'
import type { ChatReply, ChatState, FreeSlot, Lang, Message, Project } from '../api/types'
import SlotPicker from '../components/SlotPicker'
import VisitPass from '../components/VisitPass'
import { Loading } from '../components/ui'
import { addDays, isoDay, time } from '../lib/format'
import { errorText, useApi } from '../lib/useApi'
import { photos } from './catalogue'
import { languageNames, shortDay, words } from './words'
import './chat.css'

// The buyer's chat with the assistant. One chat per browser: the server keeps
// it behind an HttpOnly cookie, so coming back resumes the same conversation.
//
//   /chat                         start or resume
//   /chat?project=..&unit=..      started from a flat's page
//   /chat?...&slot=<id>           the buyer already picked a visit time there

// A line of the chat. `slots` and `pass` pin slot cards or the visit pass to
// the reply that brought them, so they stay in place as the chat goes on.
type Line = Message & { slots?: FreeSlot[]; pass?: boolean }

export default function ChatPage() {
  const [params] = useSearchParams()
  const [chat, setChat] = useState<ChatState | null>(null)
  const [checking, setChecking] = useState(true)
  const [lines, setLines] = useState<Line[]>([])

  // Resume an existing chat if this browser has one.
  useEffect(() => {
    const controller = new AbortController()
    api<ChatState>('/chat', { signal: controller.signal })
      .then((s) => {
        setChat(s)
        setLines(s.messages)
      })
      .catch(() => {}) // 404: no chat yet, show the start form
      .finally(() => {
        if (!controller.signal.aborted) setChecking(false)
      })
    return () => controller.abort()
  }, [])

  if (checking) {
    return (
      <main className="pub-main">
        <Loading label="Opening the chat" />
      </main>
    )
  }

  return (
    <main className="pub-main chat-page">
      {chat ? (
        <Conversation chat={chat} setChat={setChat} lines={lines} setLines={setLines} />
      ) : (
        <StartForm
          projectId={params.get('project') ?? ''}
          unit={params.get('unit') ?? ''}
          slotId={params.get('slot') ?? ''}
          onStarted={(s, booked) => {
            setChat({ ...s, ...(booked ? { visit: booked.visit, handedOff: booked.handedOff, executiveName: booked.executiveName } : {}) })
            const extra: Line[] = (booked?.messages ?? []).map((m) => ({ ...m }))
            const last = [...extra].reverse().find((m) => m.role === 'assistant')
            if (last && booked?.visit) last.pass = true
            setLines([...s.messages, ...extra])
          }}
        />
      )}
    </main>
  )
}

// ---------------------------------------------------------------------------
// Starting a chat: who you are, which project, which language, and consent.
// ---------------------------------------------------------------------------

function StartForm({
  projectId,
  unit,
  slotId,
  onStarted,
}: {
  projectId: string
  unit: string
  slotId: string
  onStarted: (s: ChatState, booked?: ChatReply) => void
}) {
  const projects = useApi<{ projects: Project[] }>('/projects')
  const [lang, setLang] = useState<Lang>(() => (navigator.language.startsWith('hi') ? 'hi' : navigator.language.startsWith('gu') ? 'gu' : 'en'))
  const [project, setProject] = useState(projectId)
  const [name, setName] = useState('')
  const [phone, setPhone] = useState('')
  const [consent, setConsent] = useState(false)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const w = words[lang]

  const today = isoDay(new Date())
  const slots = useApi<{ slots: FreeSlot[] }>(slotId && projectId ? `/slots?project=${projectId}&from=${today}&to=${addDays(today, 13)}` : null)
  const picked = slots.data?.slots.find((s) => s.id === slotId)

  useEffect(() => {
    if (!project && projects.data?.projects.length) setProject(projects.data.projects[0].id)
  }, [project, projects.data])

  async function submit(e: FormEvent) {
    e.preventDefault()
    setError('')
    setBusy(true)
    try {
      const state = await api<ChatState>('/chat/start', {
        method: 'POST',
        body: { name: name.trim(), phone: phone.trim(), projectId: project, language: lang, consent },
      })
      let booked: ChatReply | undefined
      if (picked && project === projectId) {
        // The time picked on the flat's page: book it straight away.
        booked = await api<ChatReply>('/chat/book', { method: 'POST', body: { slotId: picked.id } }).catch(() => undefined)
      }
      onStarted(state, booked)
    } catch (err) {
      setError(errorText(err))
      setBusy(false)
    }
  }

  const project_ = projects.data?.projects.find((p) => p.id === project)

  return (
    <div className="chat-start">
      <aside className="chat-start-art" aria-hidden="true">
        <img src={photos.living.src} alt="" />
        <p>{project_ ? `${project_.name} · ${project_.locality}` : 'Company A · Ahmedabad'}</p>
      </aside>
      <form className="chat-start-form" onSubmit={submit} noValidate lang={lang}>
        <div className="segmented" role="group" aria-label="Language">
          {(['en', 'hi', 'gu'] as Lang[]).map((l) => (
            <button key={l} type="button" aria-pressed={lang === l} onClick={() => setLang(l)} lang={l}>
              {languageNames[l]}
            </button>
          ))}
        </div>
        <h1>{w.chatTitle}</h1>
        <p className="muted">{w.chatIntro}</p>
        {unit && <p className="chip chip-neutral">{unit}</p>}

        {picked && (
          <p className="alert alert-good">
            {w.pickedSlot(`${shortDay(lang)(picked.startsAt)}, ${time(picked.startsAt)}`, picked.executiveName)}
          </p>
        )}

        <div className="field">
          <label htmlFor="project">{w.project}</label>
          <select id="project" className="select" value={project} onChange={(e) => setProject(e.target.value)}>
            {projects.data?.projects.map((p) => (
              <option key={p.id} value={p.id}>
                {p.name} · {p.locality}
              </option>
            ))}
          </select>
        </div>
        <div className="field">
          <label htmlFor="name">{w.name}</label>
          <input id="name" className="input" autoComplete="name" value={name} onChange={(e) => setName(e.target.value)} required />
        </div>
        <div className="field">
          <label htmlFor="phone">{w.phone}</label>
          <div className="phone-row">
            <span>+91</span>
            <input
              id="phone"
              className="input"
              type="tel"
              inputMode="tel"
              autoComplete="tel-national"
              value={phone}
              onChange={(e) => setPhone(e.target.value)}
              aria-describedby="phone-hint"
              required
            />
          </div>
          <p id="phone-hint" className="field-hint">
            {w.phoneHint}
          </p>
        </div>
        <label className="check">
          <input type="checkbox" checked={consent} onChange={(e) => setConsent(e.target.checked)} />
          <span>{w.consent}</span>
        </label>

        <div role="alert" aria-live="polite">
          {error && <p className="alert alert-error">{error}</p>}
        </div>
        <button className="btn" type="submit" disabled={busy || !consent || !name.trim() || !phone.trim() || !project}>
          {busy ? (
            <>
              <span className="spinner" aria-hidden="true" /> {w.starting}
            </>
          ) : (
            w.start
          )}
        </button>
      </form>
    </div>
  )
}

// ---------------------------------------------------------------------------
// The conversation
// ---------------------------------------------------------------------------

function Conversation({
  chat,
  setChat,
  lines,
  setLines,
}: {
  chat: ChatState
  setChat: (s: ChatState) => void
  lines: Line[]
  setLines: (f: (l: Line[]) => Line[]) => void
}) {
  const w = words[chat.language] ?? words.en
  const [text, setText] = useState('')
  const [busy, setBusy] = useState<false | 'reply' | 'book' | 'person'>(false)
  const [bookingId, setBookingId] = useState('')
  const [error, setError] = useState('')
  const logRef = useRef<HTMLDivElement>(null)
  const inputRef = useRef<HTMLTextAreaElement>(null)
  const clock = useClock()
  const [deviceRef, zoom] = useFitPhone()

  // Scroll the log itself (not the page) so the newest message is in view.
  useEffect(() => {
    const log = logRef.current
    if (!log) return
    const smooth = !window.matchMedia('(prefers-reduced-motion: reduce)').matches
    log.scrollTo({ top: log.scrollHeight, behavior: smooth ? 'smooth' : 'auto' })
  }, [lines.length, busy])

  // Slot cards belong to the newest assistant message that offered them;
  // older offers are hidden once a newer reply arrives.
  const lastSlotsAt = useMemo(() => {
    for (let i = lines.length - 1; i >= 0; i--) if (lines[i].slots?.length) return i
    return -1
  }, [lines])

  function apply(reply: ChatReply) {
    const msgs: Line[] = reply.messages.map((m) => ({ ...m }))
    const lastAssistant = [...msgs].reverse().find((m) => m.role === 'assistant')
    if (lastAssistant && reply.slots.length && !reply.visit) lastAssistant.slots = reply.slots
    if (lastAssistant && reply.visit) lastAssistant.pass = true
    setLines((l) => [...l, ...msgs])
    setChat({
      ...chat,
      visit: reply.visit ?? chat.visit,
      handedOff: chat.handedOff || reply.handedOff,
      executiveName: reply.executiveName || chat.executiveName,
    })
  }

  async function send(message: string) {
    const body = message.trim()
    if (!body || busy) return
    setError('')
    setBusy('reply')
    // Show the buyer's message at once; the server's copy replaces it.
    const temp: Line = { id: 'pending', conversationId: '', role: 'user', content: body, createdAt: new Date().toISOString() }
    setLines((l) => [...l, temp])
    setText('')
    try {
      const reply = await api<ChatReply>('/chat/messages', { method: 'POST', body: { text: body } })
      setLines((l) => l.filter((m) => m.id !== 'pending'))
      apply(reply)
    } catch (err) {
      setLines((l) => l.filter((m) => m.id !== 'pending'))
      setText(body) // nothing is lost: the message goes back in the box
      setError(err instanceof ApiError && err.status === 409 ? err.message : errorText(err))
    } finally {
      setBusy(false)
      inputRef.current?.focus()
    }
  }

  async function book(slot: FreeSlot) {
    setError('')
    setBusy('book')
    setBookingId(slot.id)
    try {
      apply(await api<ChatReply>('/chat/book', { method: 'POST', body: { slotId: slot.id } }))
    } catch (err) {
      setError(errorText(err))
    } finally {
      setBusy(false)
      setBookingId('')
    }
  }

  async function person() {
    setError('')
    setBusy('person')
    try {
      apply(await api<ChatReply>('/chat/handoff', { method: 'POST' }))
    } catch (err) {
      setError(errorText(err))
    } finally {
      setBusy(false)
    }
  }

  function onKey(e: KeyboardEvent<HTMLTextAreaElement>) {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault()
      void send(text)
    }
  }

  const fewMessages = lines.filter((m) => m.role === 'user').length < 2

  // Build the rows: a date chip whenever the day changes, and each bubble
  // knows whether it starts a run from the same side (only those get a tail).
  const rows: ReactNode[] = []
  let lastDay = ''
  let lastRole = ''
  lines.forEach((m, i) => {
    if (m.role === 'system') return // staff-only notes never reach the buyer anyway
    const d = isoDay(new Date(m.createdAt))
    if (d !== lastDay) {
      rows.push(
        <p key={`day-${d}`} className="wa-chip wa-day">
          {dayLabel(m.createdAt, chat.language)}
        </p>,
      )
      lastDay = d
      lastRole = ''
    }
    const first = m.role !== lastRole
    lastRole = m.role
    const out = m.role === 'user'
    const wide = (m.pass && chat.visit) || (m.slots && i === lastSlotsAt && !chat.visit)
    rows.push(
      <div key={m.id + i} className={`wa-msg ${out ? 'wa-out' : 'wa-in'}${first ? ' is-first' : ''}${wide ? ' is-wide' : ''}`}>
        <div className="wa-text">
          {m.content}
          <span className={out ? 'wa-spacer wa-spacer-out' : 'wa-spacer'} aria-hidden="true" />
          <span className="wa-meta">
            {time(m.createdAt)}
            {out && (m.id === 'pending' ? <ClockIcon /> : <TicksIcon />)}
          </span>
        </div>
        {m.pass && chat.visit && <PassCard chat={chat} />}
        {m.slots && i === lastSlotsAt && !chat.visit && (
          <div className="msg-slots">
            <p className="msg-slots-title">{w.freeTimes}</p>
            <SlotPicker slots={m.slots} onPick={book} busyId={bookingId} disabled={!!busy} formatDay={shortDay(chat.language)} withLabel={w.withLabel} />
          </div>
        )}
      </div>,
    )
  })

  return (
    <div className="wa-stage" lang={chat.language}>
      <div className="wa-device" ref={deviceRef} style={zoom === 1 ? undefined : { zoom }}>
        <div className="wa-screen">
          <div className="wa-status" aria-hidden="true">
            <span>{clock}</span>
            <span className="wa-island" />
            <StatusIcons />
          </div>

          <header className="wa-head">
            <Link to="/" className="wa-icon-btn" aria-label="Back to homes">
              <BackIcon />
            </Link>
            <span className="wa-avatar" aria-hidden="true">
              A
            </span>
            <div className="wa-who">
              <h1>Company A</h1>
              <p aria-live="polite">{busy === 'reply' ? w.typing : w.online}</p>
            </div>
            <button
              type="button"
              className="wa-icon-btn"
              onClick={person}
              disabled={!!busy || chat.handedOff}
              aria-label={w.talkToPerson}
              title={w.talkToPerson}
            >
              <CallIcon />
            </button>
          </header>

          {chat.handedOff && (
            <p className="wa-handoff" role="status">
              {chat.executiveName ? w.handedOff(chat.executiveName) : w.handedOffTeam}
            </p>
          )}

          <div className="wa-log" ref={logRef} aria-live="polite" aria-busy={busy === 'reply'}>
            <p className="wa-chip wa-notice">
              {chat.projectName} · RERA {chat.reraNumber}
            </p>
            {rows}

            {/* A chat reopened later: the pass goes at the end. */}
            {chat.visit && !lines.some((m) => m.pass) && (
              <div className="wa-msg wa-in is-first is-wide">
                <PassCard chat={chat} />
              </div>
            )}

            {busy === 'reply' && (
              <div className="wa-msg wa-in is-first wa-typing" role="status">
                <span className="dots" aria-hidden="true">
                  <i />
                  <i />
                  <i />
                </span>
                <span className="visually-hidden">{w.thinking}</span>
              </div>
            )}
          </div>

          <div className="wa-foot">
            {error && (
              <p className="wa-chip wa-error" role="alert">
                {error}
              </p>
            )}
            {!busy && (fewMessages || !chat.handedOff) && (
              <div className="wa-quick" role="group" aria-label="Suggestions">
                {fewMessages &&
                  w.quick.map((q) => (
                    <button key={q} type="button" onClick={() => void send(q)}>
                      {q}
                    </button>
                  ))}
                {!chat.handedOff && (
                  <button type="button" onClick={person}>
                    {w.talkToPerson}
                  </button>
                )}
              </div>
            )}
            <form
              className="wa-composer"
              onSubmit={(e) => {
                e.preventDefault()
                void send(text)
              }}
            >
              <label htmlFor="msg" className="visually-hidden">
                {w.messagePlaceholder}
              </label>
              <textarea
                id="msg"
                ref={inputRef}
                rows={1}
                maxLength={1000}
                placeholder={w.messagePlaceholder}
                value={text}
                onChange={(e) => setText(e.target.value)}
                onKeyDown={onKey}
              />
              <button className="wa-send" type="submit" disabled={!!busy || !text.trim()} aria-label={w.send}>
                <SendIcon />
              </button>
            </form>
          </div>
        </div>
      </div>
    </div>
  )
}

// "Today", "Yesterday" or "Thu, 6 Nov", in the buyer's language.
function dayLabel(iso: string, lang: Lang): string {
  const d = isoDay(new Date(iso))
  const today = isoDay(new Date())
  const rtf = new Intl.RelativeTimeFormat(lang === 'en' ? 'en' : `${lang}-IN`, { numeric: 'auto' })
  const label = d === today ? rtf.format(0, 'day') : d === addDays(today, -1) ? rtf.format(-1, 'day') : shortDay(lang)(iso)
  return label.charAt(0).toUpperCase() + label.slice(1)
}

// The phone is drawn at a real phone's size (see .wa-device in chat.css) and
// shrunk as a whole to fit the window's height, so on a short laptop screen it
// keeps a phone's proportions instead of getting squat. On a real phone the
// frame is dropped (CSS) and nothing is scaled.
const DEVICE_HEIGHT = 834
const MIN_ZOOM = 0.6

function useFitPhone() {
  const ref = useRef<HTMLDivElement>(null)
  const [zoom, setZoom] = useState(1)
  useLayoutEffect(() => {
    function fit() {
      const el = ref.current
      if (!el) return
      if (window.matchMedia('(max-width: 520px)').matches) return setZoom(1)
      const top = el.parentElement!.getBoundingClientRect().top + window.scrollY
      const room = window.innerHeight - Math.min(top, 120) - 24
      setZoom(Math.min(1, Math.max(MIN_ZOOM, room / DEVICE_HEIGHT)))
    }
    fit()
    window.addEventListener('resize', fit)
    return () => window.removeEventListener('resize', fit)
  }, [])
  return [ref, zoom] as const
}

// The phone's status-bar clock, in Ahmedabad time ("7:28").
function useClock(): string {
  const read = () => time(new Date().toISOString()).replace(/\s*[ap]m$/i, '')
  const [now, setNow] = useState(read)
  useEffect(() => {
    const id = window.setInterval(() => setNow(read()), 15_000)
    return () => window.clearInterval(id)
  }, [])
  return now
}

// ---- Icons (drawn here; 24px grid, currentColor) ----

const BackIcon = () => (
  <svg viewBox="0 0 24 24" width="22" height="22" aria-hidden="true">
    <path d="M15.5 4.5 8 12l7.5 7.5" fill="none" stroke="currentColor" strokeWidth="2.2" strokeLinecap="round" strokeLinejoin="round" />
  </svg>
)
const CallIcon = () => (
  <svg viewBox="0 0 24 24" width="21" height="21" aria-hidden="true">
    <path
      d="M6.6 10.8a15.1 15.1 0 0 0 6.6 6.6l2.2-2.2c.3-.3.7-.4 1-.2 1.1.4 2.3.6 3.6.6.6 0 1 .4 1 1V20c0 .6-.4 1-1 1A17 17 0 0 1 3 4c0-.6.4-1 1-1h3.5c.6 0 1 .4 1 1 0 1.3.2 2.5.6 3.6.1.3 0 .7-.2 1l-2.3 2.2Z"
      fill="currentColor"
    />
  </svg>
)
const SendIcon = () => (
  <svg viewBox="0 0 24 24" width="22" height="22" aria-hidden="true">
    <path d="M3.4 20.4 21 12 3.4 3.6 3.4 10l12.6 2-12.6 2z" fill="currentColor" />
  </svg>
)
const TicksIcon = () => (
  <svg className="wa-ticks" role="img" viewBox="0 0 18 11" width="17" height="11" aria-label="read">
    <path d="M1 5.8 4.2 9 11 1.8M7.2 8.4 7.8 9 14.6 1.8" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round" />
  </svg>
)
const ClockIcon = () => (
  <svg className="wa-pending" role="img" viewBox="0 0 16 16" width="12" height="12" aria-label="sending">
    <circle cx="8" cy="8" r="6.2" fill="none" stroke="currentColor" strokeWidth="1.4" />
    <path d="M8 4.6V8l2.2 1.4" fill="none" stroke="currentColor" strokeWidth="1.4" strokeLinecap="round" />
  </svg>
)
const StatusIcons = () => (
  <span className="wa-status-icons">
    <svg viewBox="0 0 18 12" width="17" height="11">
      <rect x="0" y="8" width="3" height="4" rx="1" fill="currentColor" />
      <rect x="5" y="5.5" width="3" height="6.5" rx="1" fill="currentColor" />
      <rect x="10" y="3" width="3" height="9" rx="1" fill="currentColor" />
      <rect x="15" y="0" width="3" height="12" rx="1" fill="currentColor" />
    </svg>
    <svg viewBox="0 0 16 12" width="15" height="11">
      <path d="M8 11.5 5.6 9a3.4 3.4 0 0 1 4.8 0L8 11.5Z M3.4 6.8a6.6 6.6 0 0 1 9.2 0l-1.2 1.2a4.9 4.9 0 0 0-6.8 0L3.4 6.8Z M1.2 4.6a9.7 9.7 0 0 1 13.6 0l-1.2 1.2a8 8 0 0 0-11.2 0L1.2 4.6Z" fill="currentColor" />
    </svg>
    <svg viewBox="0 0 27 12" width="25" height="11">
      <rect x="0.5" y="0.5" width="22" height="11" rx="3" fill="none" stroke="currentColor" opacity="0.45" />
      <rect x="2" y="2" width="16" height="8" rx="1.8" fill="currentColor" />
      <path d="M24.5 4v4c.8-.3 1.5-1 1.5-2s-.7-1.7-1.5-2Z" fill="currentColor" opacity="0.45" />
    </svg>
  </span>
)

function PassCard({ chat }: { chat: ChatState }) {
  const w = words[chat.language] ?? words.en
  if (!chat.visit) return null
  return (
    <div className="msg-pass">
      <p className="msg-slots-title">{w.bookedTitle}</p>
      <VisitPass visit={chat.visit} words={w.pass} />
      <Link className="btn btn-quiet btn-small" to={`/visit/${chat.visit.code}`}>
        {w.openVisit}
      </Link>
    </div>
  )
}
