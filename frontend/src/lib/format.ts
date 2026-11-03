// Formatting helpers. Every date is shown in Ahmedabad time, whatever time zone
// the viewer's computer is in, because slots and visits happen in Ahmedabad.

const TZ = 'Asia/Kolkata'

const dayFmt = new Intl.DateTimeFormat('en-IN', { timeZone: TZ, weekday: 'short', day: 'numeric', month: 'short' })
const longDayFmt = new Intl.DateTimeFormat('en-IN', { timeZone: TZ, weekday: 'long', day: 'numeric', month: 'long' })
const timeFmt = new Intl.DateTimeFormat('en-IN', { timeZone: TZ, hour: 'numeric', minute: '2-digit', hour12: true })
const dateTimeFmt = new Intl.DateTimeFormat('en-IN', {
  timeZone: TZ,
  day: 'numeric',
  month: 'short',
  hour: 'numeric',
  minute: '2-digit',
  hour12: true,
})
const isoDayFmt = new Intl.DateTimeFormat('en-CA', { timeZone: TZ, year: 'numeric', month: '2-digit', day: '2-digit' })

/** "Thu, 6 Nov" */
export const day = (iso: string) => dayFmt.format(new Date(iso))
/** "Thursday, 6 November" */
export const longDay = (iso: string) => longDayFmt.format(new Date(iso))
/** "11:00 am" */
export const time = (iso: string) => timeFmt.format(new Date(iso))
/** "6 Nov, 11:00 am" */
export const dateTime = (iso: string) => dateTimeFmt.format(new Date(iso))
/** "2026-11-06" in Ahmedabad time: the format the API takes for dates. */
export const isoDay = (d: Date) => isoDayFmt.format(d)

/** Adds whole days to a YYYY-MM-DD string. */
export function addDays(isoDate: string, days: number): string {
  const d = new Date(isoDate + 'T12:00:00+05:30')
  d.setUTCDate(d.getUTCDate() + days)
  return isoDay(d)
}

/** "2 min", "3 h 10 min", "2 days" */
export function duration(seconds: number | null | undefined): string {
  if (seconds == null) return '–'
  if (seconds < 60) return `${Math.round(seconds)} s`
  const min = Math.round(seconds / 60)
  if (min < 60) return `${min} min`
  const h = Math.floor(min / 60)
  if (h < 48) return min % 60 ? `${h} h ${min % 60} min` : `${h} h`
  return `${Math.round(h / 24)} days`
}

/** "₹16,500", or "–" when unknown. */
export function rupees(n: number | null | undefined): string {
  if (n == null) return '–'
  return '₹' + Math.round(n).toLocaleString('en-IN')
}

/** "6.5%", or "–" when unknown (nothing to divide by). */
export function pct(n: number | null | undefined, digits = 1): string {
  if (n == null) return '–'
  return `${n.toFixed(digits).replace(/\.0$/, '')}%`
}

/** "+91 98765 43210" */
export function phone(e164: string): string {
  const m = /^\+91(\d{5})(\d{5})$/.exec(e164)
  return m ? `+91 ${m[1]} ${m[2]}` : e164
}

/** "3 hours ago", "in 2 days" */
export function relative(iso: string, now = Date.now()): string {
  const diff = new Date(iso).getTime() - now
  const abs = Math.abs(diff)
  const rtf = new Intl.RelativeTimeFormat('en', { numeric: 'auto' })
  if (abs < 3600_000) return rtf.format(Math.round(diff / 60_000), 'minute')
  if (abs < 48 * 3600_000) return rtf.format(Math.round(diff / 3600_000), 'hour')
  return rtf.format(Math.round(diff / 86_400_000), 'day')
}

/** Google Maps link for a location pin (opens the Maps app on phones). */
export const mapsLink = (lat: number, lng: number) => `https://www.google.com/maps/search/?api=1&query=${lat},${lng}`
