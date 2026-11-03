import { useEffect, useMemo, useRef, useState, type FormEvent, type KeyboardEvent } from 'react'
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
  const endRef = useRef<HTMLDivElement>(null)
  const inputRef = useRef<HTMLTextAreaElement>(null)

  useEffect(() => {
    const smooth = !window.matchMedia('(prefers-reduced-motion: reduce)').matches
    endRef.current?.scrollIntoView({ behavior: smooth ? 'smooth' : 'auto', block: 'end' })
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

  return (
    <div className="chat" lang={chat.language}>
      <header className="chat-head">
        <div>
          <p className="eyebrow">{chat.projectName}</p>
          <h1>{w.chatTitle}</h1>
        </div>
        <button className="btn btn-quiet btn-small" onClick={person} disabled={!!busy || chat.handedOff}>
          {w.talkToPerson}
        </button>
      </header>

      {chat.handedOff && (
        <p className="chat-banner" role="status">
          {chat.executiveName ? w.handedOff(chat.executiveName) : w.handedOffTeam}
        </p>
      )}

      <div className="chat-log" aria-live="polite" aria-busy={busy === 'reply'}>
        {lines.map((m, i) => (
          <div key={m.id + i} className={`msg msg-${m.role}`}>
            <p>{m.content}</p>
            {m.pass && chat.visit && <PassCard chat={chat} />}
            {m.slots && i === lastSlotsAt && !chat.visit && (
              <div className="msg-slots">
                <p className="msg-slots-title">{w.freeTimes}</p>
                <SlotPicker slots={m.slots} onPick={book} busyId={bookingId} disabled={!!busy} formatDay={shortDay(chat.language)} withLabel={w.withLabel} />
              </div>
            )}
          </div>
        ))}

        {/* A chat reopened later: the pass goes at the end. */}
        {chat.visit && !lines.some((m) => m.pass) && (
          <div className="msg msg-assistant">
            <PassCard chat={chat} />
          </div>
        )}

        {busy === 'reply' && (
          <div className="msg msg-assistant msg-typing" role="status">
            <span className="dots" aria-hidden="true">
              <i />
              <i />
              <i />
            </span>
            <span className="visually-hidden">{w.thinking}</span>
          </div>
        )}
        <div ref={endRef} />
      </div>

      <div className="chat-foot">
        {error && (
          <p className="alert alert-error" role="alert">
            {error}
          </p>
        )}
        {fewMessages && !busy && (
          <div className="quick" role="group" aria-label="Suggestions">
            {w.quick.map((q) => (
              <button key={q} type="button" className="fchip" onClick={() => void send(q)}>
                {q}
              </button>
            ))}
          </div>
        )}
        <form
          className="composer"
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
          <button className="btn" type="submit" disabled={!!busy || !text.trim()}>
            {w.send}
          </button>
        </form>
        <p className="chat-rera">
          {chat.projectName} · RERA {chat.reraNumber}
        </p>
      </div>
    </div>
  )
}

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
