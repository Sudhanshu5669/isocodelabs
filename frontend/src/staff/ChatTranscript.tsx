import type { Transcript } from '../api/types'
import { Chip } from '../components/ui'
import { dateTime, time } from '../lib/format'
import { languageLabel } from '../lib/labels'

// One AI chat as staff see it: the summary written at handoff, why it was
// flagged (if it was), and every message. Lines marked "Staff note" were never
// shown to the buyer (e.g. a reply the guardrail blocked).

export default function ChatTranscript({ chat, open = false }: { chat: Transcript; open?: boolean }) {
  return (
    <article className="transcript" id={`chat-${chat.id}`}>
      <header className="transcript-head">
        <div>
          <h3>Website chat</h3>
          <p className="muted small">
            Started {dateTime(chat.createdAt)} · {languageLabel[chat.language] ?? chat.language}
            {chat.handedOffAt && <> · Handed to a person {dateTime(chat.handedOffAt)}</>}
          </p>
        </div>
        <div className="row-flags">
          {chat.flagged && <Chip tone="bad">Flagged for review</Chip>}
          {chat.handedOffAt && <Chip tone="warn">Handed off</Chip>}
        </div>
      </header>

      {chat.flagged && chat.flagReason && (
        <p className="alert alert-error small">
          <strong>Why it was flagged:</strong> {chat.flagReason}
        </p>
      )}

      {chat.summary ? (
        <div className="summary">
          <p className="eyebrow">Summary for the salesperson</p>
          <p className="summary-text">{chat.summary}</p>
        </div>
      ) : chat.handedOffAt ? (
        <p className="muted small">The summary is still being written. Reload in a moment.</p>
      ) : null}

      <details className="transcript-body" open={open}>
        <summary>
          {chat.messages.length} message{chat.messages.length === 1 ? '' : 's'}
        </summary>
        <ol className="messages">
          {chat.messages.map((m, i) => (
            <li key={i} className={`msg msg-${m.role}`}>
              <span className="msg-who">
                {m.role === 'user' ? 'Buyer' : m.role === 'assistant' ? 'Assistant' : 'Staff note'} ·{' '}
                {time(m.createdAt)}
              </span>
              <p>{m.content}</p>
            </li>
          ))}
        </ol>
      </details>
    </article>
  )
}
