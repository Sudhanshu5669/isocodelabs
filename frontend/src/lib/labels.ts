// The words staff see for the API's machine values, in one place, so a status
// is called the same thing on every screen.

import type { LeadStatus, Source, VisitStatus } from '../api/types'

export const sourceLabel: Record<Source, string> = {
  property_portal: 'Property portal',
  social_ad: 'Social media ad',
  website: 'Website chat',
  hoarding_call: 'Hoarding call',
  manual: 'Added by staff',
}

export const leadStatusLabel: Record<LeadStatus, string> = {
  new: 'New',
  engaged: 'In conversation',
  visit_booked: 'Visit booked',
  visited: 'Visited',
  lost: 'Lost',
}

export const visitStatusLabel: Record<VisitStatus, string> = {
  booked: 'Booked',
  visited: 'Visited',
  no_show: 'No-show',
  cancelled: 'Cancelled',
}

export const timelineLabel: Record<string, string> = {
  within_3_months: 'Within 3 months',
  '3_to_6_months': 'In 3 to 6 months',
  '6_to_12_months': 'In 6 to 12 months',
  over_12_months: 'After a year',
  exploring: 'Just exploring',
}

export const languageLabel: Record<string, string> = { en: 'English', hi: 'Hindi', gu: 'Gujarati' }

export const reminderKindLabel: Record<string, string> = {
  day_before: '1 day before',
  two_hours_before: '2 hours before',
}

export const reminderReasonLabel: Record<string, string> = {
  opted_out: 'Buyer opted out or gave no consent',
  too_late: 'Too late to be useful',
  visit_cancelled: 'Visit was cancelled',
  visit_not_booked: 'Visit no longer booked',
  send_failed: 'Message could not be delivered',
}

/** Tone of a status chip: calm by default, green for good news, amber for "act". */
export type Tone = 'neutral' | 'good' | 'warn' | 'bad' | 'quiet'

export const leadStatusTone: Record<LeadStatus, Tone> = {
  new: 'warn',
  engaged: 'neutral',
  visit_booked: 'good',
  visited: 'good',
  lost: 'quiet',
}

export const visitStatusTone: Record<VisitStatus, Tone> = {
  booked: 'good',
  visited: 'good',
  no_show: 'bad',
  cancelled: 'quiet',
}
