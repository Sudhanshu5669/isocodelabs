import type { ReactNode } from 'react'
import type { Tone } from '../lib/labels'

// Small building blocks every screen uses, so loading, errors and empty lists
// look and read the same everywhere.

export function Chip({ tone = 'neutral', children }: { tone?: Tone; children: ReactNode }) {
  return <span className={`chip chip-${tone}`}>{children}</span>
}

export function Loading({ label = 'Loading' }: { label?: string }) {
  return (
    <div className="state" role="status">
      <span className="spinner" aria-hidden="true" />
      <span className="visually-hidden">{label}</span>
    </div>
  )
}

/** Shown when a load failed: says what happened and offers a retry. */
export function ErrorState({ message, onRetry }: { message: string; onRetry?: () => void }) {
  return (
    <div className="state" role="alert">
      <h2>This did not load</h2>
      <p>{message}</p>
      {onRetry && (
        <button className="btn btn-quiet btn-small" onClick={onRetry}>
          Try again
        </button>
      )}
    </div>
  )
}

/** Shown when there is nothing to list. Says what would appear, and how. */
export function EmptyState({ title, children }: { title: string; children?: ReactNode }) {
  return (
    <div className="state">
      <h2>{title}</h2>
      {children && <p>{children}</p>}
    </div>
  )
}

export function PageHead({ title, intro, children }: { title: string; intro?: ReactNode; children?: ReactNode }) {
  return (
    <header className="page-head">
      <div>
        <h1>{title}</h1>
        {intro && <p>{intro}</p>}
      </div>
      {children && <div className="actions">{children}</div>}
    </header>
  )
}

/** A short-lived message after an action ("Saved", "Visit cancelled"). */
export function Notice({ kind, children }: { kind: 'good' | 'error' | 'warn'; children: ReactNode }) {
  return (
    <p className={`alert alert-${kind}`} role={kind === 'error' ? 'alert' : 'status'}>
      {children}
    </p>
  )
}
