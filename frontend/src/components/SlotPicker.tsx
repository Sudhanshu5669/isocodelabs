import type { FreeSlot } from '../api/types'
import { day as formatDay, time } from '../lib/format'
import './slot-picker.css'

// Free visit times, grouped by day. Used in the chat (slot cards), on the
// buyer's visit page (reschedule) and on the lead page (book for a buyer).
// The times always come from the server's list of open slots: nothing here
// decides what is free.

type Props = {
  slots: FreeSlot[]
  onPick: (slot: FreeSlot) => void
  /** The slot being booked right now, shown with a spinner. */
  busyId?: string
  disabled?: boolean
  /** Formats the day heading; lets the buyer pages use their own language. */
  formatDay?: (iso: string) => string
  withLabel?: string
}

export default function SlotPicker({ slots, onPick, busyId, disabled, formatDay: fmt = formatDay, withLabel = 'with' }: Props) {
  const groups: { day: string; slots: FreeSlot[] }[] = []
  for (const s of slots) {
    const d = fmt(s.startsAt)
    const last = groups[groups.length - 1]
    if (last && last.day === d) last.slots.push(s)
    else groups.push({ day: d, slots: [s] })
  }

  return (
    <div className="slots">
      {groups.map((g) => (
        <div key={g.day} className="slots-day">
          <p className="slots-day-name">{g.day}</p>
          <div className="slots-row">
            {g.slots.map((s) => (
              <button
                key={s.id}
                type="button"
                className="slot"
                onClick={() => onPick(s)}
                disabled={disabled || !!busyId}
                aria-busy={busyId === s.id}
              >
                <span className="slot-time num">
                  {busyId === s.id && <span className="spinner" aria-hidden="true" />}
                  {time(s.startsAt)}
                </span>
                <span className="slot-who">
                  {withLabel} {s.executiveName.split(' ')[0]}
                </span>
              </button>
            ))}
          </div>
        </div>
      ))}
    </div>
  )
}
