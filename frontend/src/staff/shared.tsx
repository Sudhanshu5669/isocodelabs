import { useState, type ReactNode } from 'react'
import type { Project, StaffUser } from '../api/types'
import { useAuth } from '../auth/AuthContext'
import { useApi } from '../lib/useApi'

// Small pieces several staff screens share.

/** Every project (the public list; the same two everywhere). */
export function useProjects() {
  const res = useApi<{ projects: Project[] }>('/projects')
  return res.data?.projects ?? []
}

/**
 * The team, for "assign to" and "slot for" pickers. Only the head may list
 * users, so for an executive this loads nothing and returns an empty list.
 */
export function useTeam(): StaffUser[] {
  const { user } = useAuth()
  const res = useApi<{ users: StaffUser[] }>(user?.role === 'head' ? '/staff/users' : null)
  return res.data?.users ?? []
}

/**
 * A button that asks "are you sure?" in place before doing something that
 * cannot be undone, instead of a browser pop-up that blocks the page.
 */
export function ConfirmButton({
  children,
  question,
  confirmLabel = 'Yes',
  onConfirm,
  danger,
  disabled,
}: {
  children: ReactNode
  question: string
  confirmLabel?: string
  onConfirm: () => unknown
  danger?: boolean
  disabled?: boolean
}) {
  const [asking, setAsking] = useState(false)
  const [busy, setBusy] = useState(false)

  if (!asking) {
    return (
      <button type="button" className="btn btn-quiet btn-small" onClick={() => setAsking(true)} disabled={disabled}>
        {children}
      </button>
    )
  }
  return (
    <span className="confirm" role="group" aria-label={question}>
      <span className="confirm-q">{question}</span>
      <button
        type="button"
        className={`btn btn-small ${danger ? 'btn-danger' : ''}`}
        disabled={busy}
        onClick={async () => {
          setBusy(true)
          try {
            await onConfirm()
          } finally {
            setBusy(false)
            setAsking(false)
          }
        }}
      >
        {busy && <span className="spinner" aria-hidden="true" />}
        {confirmLabel}
      </button>
      <button type="button" className="btn btn-quiet btn-small" onClick={() => setAsking(false)} disabled={busy}>
        No
      </button>
    </span>
  )
}

/** A label/value pair in a details list. */
export function Detail({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="detail">
      <dt>{label}</dt>
      <dd>{children}</dd>
    </div>
  )
}

/** "₹65 lakh to ₹80 lakh", "From ₹70 lakh", or null when nothing is known. */
export function budgetText(min: number | null, max: number | null): string | null {
  const l = (n: number) => (n >= 100 ? `₹${(n / 100).toFixed(2).replace(/\.?0+$/, '')} crore` : `₹${n} lakh`)
  if (min != null && max != null) return min === max ? l(min) : `${l(min)} to ${l(max)}`
  if (min != null) return `From ${l(min)}`
  if (max != null) return `Up to ${l(max)}`
  return null
}
