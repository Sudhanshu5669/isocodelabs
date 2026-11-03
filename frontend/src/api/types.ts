// The shapes the Go API sends back. Each one mirrors a Go struct (named in the
// comment), so when the backend changes, this is the one file to update.

export type Lang = 'en' | 'hi' | 'gu'

// models.Project
export type Project = {
  id: string
  name: string
  reraNumber: string
  locality: string
  address: string
  latitude: number
  longitude: number
}

// GET /api/projects/{id}
export type ProjectWithFacts = Project & { facts: { topic: string; label: string; value: string }[] }

// models.FactSheetEntry
export type Fact = {
  id: string
  projectId: string
  topic: string
  label: string
  value: string
  updatedBy: string | null
  updatedAt: string
}

// store.FreeSlot (public slot list) and assistant.Slot (slot cards in the chat)
export type FreeSlot = {
  id: string
  projectId?: string
  startsAt: string
  endsAt: string
  executiveName: string
}

// store.VisitDetail: the buyer's visit page
export type VisitDetail = {
  id: string
  code: string
  status: VisitStatus
  slotId: string
  startsAt: string
  endsAt: string
  projectId: string
  projectName: string
  reraNumber: string
  address: string
  latitude: number
  longitude: number
  executiveName: string
  leadFirstName: string
  canChange: boolean
  optedOut: boolean
}

export type VisitStatus = 'booked' | 'visited' | 'no_show' | 'cancelled'

// models.Message
export type Message = {
  id: string
  conversationId: string
  role: 'user' | 'assistant' | 'system'
  content: string
  createdAt: string
}

// httpapi.ChatState
export type ChatState = {
  firstName: string
  language: Lang
  projectId: string
  projectName: string
  reraNumber: string
  messages: Message[]
  handedOff: boolean
  executiveName?: string
  visit: VisitDetail | null
}

// httpapi.ChatReply
export type ChatReply = {
  messages: Message[]
  slots: FreeSlot[]
  visit: VisitDetail | null
  handedOff: boolean
  executiveName?: string
}

// ---- staff -----------------------------------------------------------------

export type LeadStatus = 'new' | 'engaged' | 'visit_booked' | 'visited' | 'lost'
export type Source = 'property_portal' | 'social_ad' | 'website' | 'hoarding_call' | 'manual'

// store.LeadRow
export type LeadRow = {
  id: string
  code: string
  name: string
  phone: string
  source: Source
  projectId: string | null
  projectName: string | null
  wants: string | null
  status: LeadStatus
  language: Lang
  consent: boolean
  optedOut: boolean
  assignedTo: string | null
  assigneeName: string | null
  firstResponseAt: string | null
  firstResponseSeconds: number | null
  duplicateOfCode: string | null
  createdAt: string
}

// store.LeadPage
export type LeadPage = { leads: LeadRow[]; total: number }

// models.LeadTouch
export type Touch = {
  id: string
  leadId: string
  source: string
  projectId: string | null
  note: string | null
  createdAt: string
}

// store.ChatTranscript
export type Transcript = {
  id: string
  projectId: string | null
  language: Lang
  summary: string | null
  handedOffAt: string | null
  flagged: boolean
  flagReason: string | null
  createdAt: string
  messages: { role: 'user' | 'assistant' | 'system'; content: string; createdAt: string }[]
}

// store.LeadDetail
export type LeadDetail = LeadRow & {
  budgetMinLakh: number | null
  budgetMaxLakh: number | null
  timeline: string | null
  consentAt: string | null
  optedOutAt: string | null
  duplicateOf: { id: string; code: string; name: string } | null
  touches: Touch[]
  visit: {
    id: string
    code: string
    status: VisitStatus
    startsAt: string
    projectName: string
    executiveName: string
  } | null
  chats: Transcript[]
}

// store.StaffVisit
export type StaffVisit = {
  id: string
  code: string
  status: VisitStatus
  startsAt: string
  endsAt: string
  projectId: string
  projectName: string
  executiveId: string
  executiveName: string
  leadId: string
  leadCode: string
  leadName: string
  leadPhone: string
  canMark: boolean
}

// store.StaffSlot
export type StaffSlot = {
  id: string
  projectId: string
  projectName: string
  executiveId: string
  executiveName: string
  startsAt: string
  endsAt: string
  status: 'open' | 'booked' | 'cancelled'
  visitId: string | null
  leadName: string | null
}

// store.ReminderLogEntry
export type ReminderEntry = {
  id: string
  kind: 'day_before' | 'two_hours_before'
  dueAt: string
  status: 'queued' | 'sent' | 'skipped' | 'failed'
  skipReason: string | null
  body: string | null
  sentAt: string | null
  visitId: string
  visitStartsAt: string
  leadId: string
  leadCode: string
  leadName: string
  projectName: string
  executiveName: string
}

// store.FlaggedChat
export type FlaggedChat = {
  id: string
  leadId: string
  leadCode: string
  leadName: string
  flagReason: string
  updatedAt: string
}

// store.StaffUser
export type StaffUser = {
  id: string
  email: string
  name: string
  role: 'executive' | 'head'
  active: boolean
  createdAt: string
  openLeads: number
  upcomingVisits: number
}

// store.DeactivateResult
export type DeactivateResult = { cancelledOpenSlots: number; bookedVisits: number; assignedLeads: number }

// store.Metrics
export type Metrics = {
  source?: Source
  leads: number
  responded: number
  medianFirstResponseSeconds: number | null
  respondedWithin5Min: number
  respondedWithin5MinPct: number | null
  oldEnoughFor48h: number
  unreachedIn48h: number
  unreachedIn48hPct: number | null
  leadsWithVisitBooked: number
  leadsVisited: number
  leadToVisitPct: number | null
  visitsDone: number
  noShows: number
  showUpRatePct: number | null
  spendInr: number | null
  costPerLeadInr: number | null
  costPerVisitInr: number | null
}

// store.Dashboard
export type Dashboard = {
  from: string
  to: string
  projectId?: string
  totals: Metrics
  bySource: Metrics[]
  daily: { date: string; leads: number; medianFirstResponseSeconds: number | null }[]
  spendNote?: string
}

// models.SourceSpend
export type Spend = { id: string; source: Source; month: string; amountInr: number }
