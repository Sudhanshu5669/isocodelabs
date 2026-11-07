import { useEffect, useState } from 'react'
import { Link, useNavigate, useSearchParams } from 'react-router-dom'
import type { LeadPage, LeadStatus, Source } from '../api/types'
import { useAuth } from '../auth/AuthContext'
import { Chip, EmptyState, ErrorState, PageHead } from '../components/ui'
import { duration, phone, relative } from '../lib/format'
import { leadStatusLabel, leadStatusTone, sourceLabel } from '../lib/labels'
import { useApi } from '../lib/useApi'
import AddLeadForm from './AddLeadForm'
import { useProjects, useTeam } from './shared'
import './staff.css'

// The one lead list: every source in one place, with source tags, the first
// reply time and possible duplicates flagged. Filters live in the URL, so a
// filtered list can be bookmarked and the back button works.

const PAGE = 25

export default function LeadsPage() {
  const { user } = useAuth()
  const isHead = user?.role === 'head'
  const navigate = useNavigate()
  const projects = useProjects()
  const team = useTeam()
  const [params, setParams] = useSearchParams()
  const [adding, setAdding] = useState(false)

  const page = Math.max(0, Number(params.get('page') ?? '0') || 0)
  const [search, setSearch] = useState(params.get('q') ?? '')

  // Search as you type, but wait for a pause so each key press is not a request.
  useEffect(() => {
    const t = setTimeout(() => {
      if ((params.get('q') ?? '') !== search.trim()) update({ q: search.trim() })
    }, 300)
    return () => clearTimeout(t)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [search])

  function update(changes: Record<string, string>) {
    const next = new URLSearchParams(params)
    for (const [k, v] of Object.entries(changes)) {
      if (v) next.set(k, v)
      else next.delete(k)
    }
    if (!('page' in changes)) next.delete('page') // a new filter starts at page 1
    setParams(next, { replace: true })
  }

  const query = new URLSearchParams()
  for (const k of ['q', 'status', 'source', 'project', 'assignee']) {
    const v = params.get(k)
    if (v) query.set(k, v)
  }
  if (params.get('attention') === '1') query.set('needsAttention', 'true')
  query.set('limit', String(PAGE))
  query.set('offset', String(page * PAGE))
  const res = useApi<LeadPage>(`/staff/leads?${query}`, { keepPrevious: true })

  const total = res.data?.total ?? 0
  const filtered = ['q', 'status', 'source', 'project', 'assignee', 'attention'].some((k) => params.get(k))

  return (
    <section>
      <PageHead
        title="Leads"
        intro={
          isHead
            ? 'Every enquiry from every source. Possible duplicates are flagged for you to merge or keep apart.'
            : 'The leads assigned to you, newest first.'
        }
      >
        <button className="btn btn-small" onClick={() => setAdding((a) => !a)} aria-expanded={adding}>
          {adding ? 'Close' : 'Add a lead'}
        </button>
      </PageHead>

      {adding && (
        <AddLeadForm
          projects={projects}
          onDone={(leadId) => {
            setAdding(false)
            res.reload()
            if (leadId) navigate(`/staff/leads/${leadId}`)
          }}
        />
      )}

      <div className="filters" role="search">
        <input
          className="input filters-search"
          type="search"
          placeholder="Search a name, full phone number or LD-code"
          aria-label="Search leads"
          value={search}
          maxLength={80}
          onChange={(e) => setSearch(e.target.value)}
        />
        <select
          className="select"
          aria-label="Status"
          value={params.get('status') ?? ''}
          onChange={(e) => update({ status: e.target.value })}
        >
          <option value="">Any status</option>
          {(Object.keys(leadStatusLabel) as LeadStatus[]).map((s) => (
            <option key={s} value={s}>
              {leadStatusLabel[s]}
            </option>
          ))}
        </select>
        <select
          className="select"
          aria-label="Source"
          value={params.get('source') ?? ''}
          onChange={(e) => update({ source: e.target.value })}
        >
          <option value="">Every source</option>
          {(Object.keys(sourceLabel) as Source[]).map((s) => (
            <option key={s} value={s}>
              {sourceLabel[s]}
            </option>
          ))}
        </select>
        <select
          className="select"
          aria-label="Project"
          value={params.get('project') ?? ''}
          onChange={(e) => update({ project: e.target.value })}
        >
          <option value="">Both projects</option>
          {projects.map((p) => (
            <option key={p.id} value={p.id}>
              {p.name}
            </option>
          ))}
        </select>
        {isHead && (
          <select
            className="select"
            aria-label="Assigned to"
            value={params.get('assignee') ?? ''}
            onChange={(e) => update({ assignee: e.target.value })}
          >
            <option value="">Anyone</option>
            <option value="unassigned">Not assigned</option>
            {team
              .filter((u) => u.role === 'executive')
              .map((u) => (
                <option key={u.id} value={u.id}>
                  {u.name}
                </option>
              ))}
          </select>
        )}
        <label className="check filters-check">
          <input
            type="checkbox"
            checked={params.get('attention') === '1'}
            onChange={(e) => update({ attention: e.target.checked ? '1' : '' })}
          />
          Needs attention
        </label>
        {filtered && (
          <button
            className="btn-link small"
            onClick={() => {
              setSearch('')
              setParams(new URLSearchParams(), { replace: true })
            }}
          >
            Clear filters
          </button>
        )}
      </div>

      {res.error ? (
        <ErrorState message={res.error} onRetry={res.reload} />
      ) : res.loading && !res.data ? (
        <TableSkeleton />
      ) : total === 0 ? (
        <div className="table-wrap">
          {filtered ? (
            <EmptyState title="No leads match">Try fewer filters, or search by phone number or LD-code.</EmptyState>
          ) : (
            <EmptyState title="No leads yet">
              {isHead
                ? 'Enquiries from the portals, ads, hoardings and the website chat will appear here.'
                : 'When the sales head assigns you a lead, or a buyer you were handed by the assistant writes in, it appears here.'}
            </EmptyState>
          )}
        </div>
      ) : (
        <>
          <div className="table-wrap" aria-busy={res.loading}>
            <table className="table table-cards">
              <thead>
                <tr>
                  <th>Lead</th>
                  <th>Source</th>
                  <th>Looking for</th>
                  <th>Status</th>
                  <th>First reply</th>
                  {isHead && <th>Assigned to</th>}
                  <th>Came in</th>
                </tr>
              </thead>
              <tbody>
                {res.data!.leads.map((l) => (
                  <tr key={l.id} className="row-link" onClick={() => navigate(`/staff/leads/${l.id}`)}>
                    <td>
                      <Link className="row-title" to={`/staff/leads/${l.id}`} onClick={(e) => e.stopPropagation()}>
                        {l.name}
                      </Link>
                      <div className="muted small num">
                        {l.code} · {phone(l.phone)}
                      </div>
                      <div className="row-flags">
                        {l.duplicateOfCode && <Chip tone="warn">Possible duplicate of {l.duplicateOfCode}</Chip>}
                        {l.optedOut && <Chip tone="bad">Opted out</Chip>}
                      </div>
                    </td>
                    <td data-label="Source">{sourceLabel[l.source]}</td>
                    <td data-label="Looking for">
                      {[l.projectName, l.wants].filter(Boolean).join(' · ') || <span className="muted">Not known</span>}
                    </td>
                    <td data-label="Status">
                      <Chip tone={leadStatusTone[l.status]}>{leadStatusLabel[l.status]}</Chip>
                    </td>
                    <td data-label="First reply" className="num">
                      {l.firstResponseSeconds == null ? (
                        <span className="text-warn">Not yet</span>
                      ) : (
                        <span className={l.firstResponseSeconds > 300 ? 'text-warn' : ''}>
                          {duration(l.firstResponseSeconds)}
                        </span>
                      )}
                    </td>
                    {isHead && (
                      <td data-label="Assigned to">{l.assigneeName ?? <span className="text-warn">Not assigned</span>}</td>
                    )}
                    <td data-label="Came in" className="muted">
                      {relative(l.createdAt)}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
          <div className="pager">
            <span className="muted small num">
              {page * PAGE + 1}–{Math.min(total, (page + 1) * PAGE)} of {total}
            </span>
            <div className="actions">
              <button
                className="btn btn-quiet btn-small"
                disabled={page === 0}
                onClick={() => update({ page: String(page - 1) })}
              >
                Previous
              </button>
              <button
                className="btn btn-quiet btn-small"
                disabled={(page + 1) * PAGE >= total}
                onClick={() => update({ page: String(page + 1) })}
              >
                Next
              </button>
            </div>
          </div>
        </>
      )}
    </section>
  )
}

function TableSkeleton() {
  return (
    <div className="table-wrap skeleton-list" role="status">
      <span className="visually-hidden">Loading leads</span>
      {Array.from({ length: 6 }, (_, i) => (
        <div key={i} className="skeleton" style={{ height: 44 }} />
      ))}
    </div>
  )
}
