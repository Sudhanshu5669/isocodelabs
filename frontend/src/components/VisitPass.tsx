import { mapsLink, time } from '../lib/format'
import './visit-pass.css'

// The visit pass: the one card a buyer keeps. It looks like an entry pass to
// the site, with the date set large, a tear line, and the project's RERA
// number on the stub (promotional material must show it). It appears in the
// chat when a visit is booked and again on the buyer's visit page.

export type PassVisit = {
  startsAt: string
  endsAt: string
  projectName: string
  address: string
  executiveName: string
  reraNumber: string
  latitude: number
  longitude: number
  status: 'booked' | 'visited' | 'no_show' | 'cancelled'
}

export type PassWords = {
  title: string
  with: string
  where: string
  openMap: string
  rera: string
  cancelled: string
  /** Long weekday and date, in the buyer's language. */
  formatDay: (iso: string) => string
}

export const englishPassWords: PassWords = {
  title: 'Site visit',
  with: 'You will meet',
  where: 'Where',
  openMap: 'Open location in Maps',
  rera: 'RERA reg. no.',
  cancelled: 'Cancelled',
  formatDay: (iso) =>
    new Intl.DateTimeFormat('en-IN', { timeZone: 'Asia/Kolkata', weekday: 'long', day: 'numeric', month: 'long' }).format(
      new Date(iso),
    ),
}

export default function VisitPass({ visit, words = englishPassWords }: { visit: PassVisit; words?: PassWords }) {
  const cancelled = visit.status === 'cancelled'
  return (
    <article className={`pass ${cancelled ? 'pass-void' : ''}`} aria-label={words.title}>
      <div className="pass-main">
        <p className="pass-kicker">
          {words.title} · {visit.projectName}
          {cancelled && <span className="pass-void-tag">{words.cancelled}</span>}
        </p>
        <p className="pass-day">{words.formatDay(visit.startsAt)}</p>
        <p className="pass-time num">
          {time(visit.startsAt)} – {time(visit.endsAt)}
        </p>
        <dl className="pass-rows">
          <div>
            <dt>{words.with}</dt>
            <dd>{visit.executiveName}</dd>
          </div>
          <div>
            <dt>{words.where}</dt>
            <dd>
              {visit.address}
              <br />
              <a href={mapsLink(visit.latitude, visit.longitude)} target="_blank" rel="noreferrer">
                {words.openMap}
              </a>
            </dd>
          </div>
        </dl>
      </div>
      <div className="pass-stub">
        <span>{words.rera}</span>
        <span className="pass-rera">{visit.reraNumber}</span>
      </div>
    </article>
  )
}
