import { useState } from 'react'
import { Link } from 'react-router-dom'
import { api } from '../api/client'
import type { FlaggedChat, LeadDetail } from '../api/types'
import { EmptyState, ErrorState, Loading, Notice, PageHead } from '../components/ui'
import { relative } from '../lib/format'
import { errorText, useApi } from '../lib/useApi'
import ChatTranscript from './ChatTranscript'
import './staff.css'

// The sales head's daily review of chats the assistant flagged: a reply the
// guardrail blocked (a price or discount the fact sheet does not support), a
// message that looked like an attempt to trick the assistant, or a buyer who
// was unhappy. Read the chat, follow up if needed, then mark it reviewed.

export default function ReviewPage() {
  const res = useApi<{ chats: FlaggedChat[] }>('/staff/chats/flagged')
  const [open, setOpen] = useState<string | null>(null)
  const [error, setError] = useState('')
  const [notice, setNotice] = useState('')

  async function markReviewed(c: FlaggedChat) {
    setError('')
    setNotice('')
    try {
      await api(`/staff/chats/${c.id}/reviewed`, { method: 'POST' })
      setNotice(`Chat with ${c.leadName} marked as reviewed.`)
      if (open === c.id) setOpen(null)
      res.reload()
    } catch (err) {
      setError(errorText(err))
    }
  }

  const chats = res.data?.chats ?? []

  return (
    <section>
      <PageHead
        title="Review"
        intro="Chats the assistant flagged for a person to read. Check each one, follow up with the buyer if needed, then mark it reviewed."
      />
      {error && <Notice kind="error">{error}</Notice>}
      {notice && <Notice kind="good">{notice}</Notice>}

      {res.error ? (
        <ErrorState message={res.error} onRetry={res.reload} />
      ) : res.loading && !res.data ? (
        <Loading label="Loading flagged chats" />
      ) : chats.length === 0 ? (
        <div className="table-wrap">
          <EmptyState title="Nothing to review">
            When the assistant blocks a reply or a buyer tries to steer it off script, the chat lands here.
          </EmptyState>
        </div>
      ) : (
        <ul className="review-list">
          {chats.map((c) => (
            <li key={c.id} className="panel review-item">
              <div className="review-head">
                <div>
                  <Link className="row-title" to={`/staff/leads/${c.leadId}`}>
                    {c.leadName}
                  </Link>
                  <span className="muted small num"> · {c.leadCode} · flagged {relative(c.updatedAt)}</span>
                  <p className="review-reason">{c.flagReason}</p>
                </div>
                <div className="actions">
                  <button
                    className="btn btn-quiet btn-small"
                    aria-expanded={open === c.id}
                    onClick={() => setOpen(open === c.id ? null : c.id)}
                  >
                    {open === c.id ? 'Hide chat' : 'Read chat'}
                  </button>
                  <button className="btn btn-small" onClick={() => markReviewed(c)}>
                    Mark reviewed
                  </button>
                </div>
              </div>
              {open === c.id && <FlaggedTranscript leadId={c.leadId} chatId={c.id} />}
            </li>
          ))}
        </ul>
      )}
    </section>
  )
}

function FlaggedTranscript({ leadId, chatId }: { leadId: string; chatId: string }) {
  const res = useApi<LeadDetail>(`/staff/leads/${leadId}`)
  if (res.error) return <ErrorState message={res.error} onRetry={res.reload} />
  if (!res.data) return <Loading label="Loading the chat" />
  const chat = res.data.chats.find((c) => c.id === chatId)
  if (!chat) return <p className="muted">This chat is no longer on the lead (it may have been merged).</p>
  return (
    <div className="review-chat">
      <ChatTranscript chat={chat} open />
    </div>
  )
}
