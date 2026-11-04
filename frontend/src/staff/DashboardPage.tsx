import { useState } from 'react'
import type { Dashboard, Metrics } from '../api/types'
import { Chip, EmptyState, ErrorState, Loading, PageHead } from '../components/ui'
import { addDays, duration, isoDay, pct, rupees } from '../lib/format'
import { sourceLabel } from '../lib/labels'
import { useApi } from '../lib/useApi'
import { DailyColumns, HorizontalBars } from './Charts'
import { useProjects } from './shared'
import './staff.css'

// The sales head's dashboard: is every enquiry answered fast, how many become
// visits, do buyers turn up, and what each visit costs by source. The targets
// are the ones in the brief.

type Period = '30d' | 'this-month' | 'last-month' | '90d'

const periods: { id: Period; label: string }[] = [
  { id: '30d', label: 'Last 30 days' },
  { id: 'this-month', label: 'This month' },
  { id: 'last-month', label: 'Last month' },
  { id: '90d', label: 'Last 90 days' },
]

function rangeFor(p: Period): { from: string; to: string } {
  const today = isoDay(new Date())
  const firstOfMonth = today.slice(0, 8) + '01'
  switch (p) {
    case 'this-month':
      return { from: firstOfMonth, to: today }
    case 'last-month': {
      const lastDay = addDays(firstOfMonth, -1)
      return { from: lastDay.slice(0, 8) + '01', to: lastDay }
    }
    case '90d':
      return { from: addDays(today, -89), to: today }
    default:
      return { from: addDays(today, -29), to: today }
  }
}

export default function DashboardPage() {
  const projects = useProjects()
  const [period, setPeriod] = useState<Period>('30d')
  const [project, setProject] = useState('')
  const { from, to } = rangeFor(period)
  const res = useApi<Dashboard>(`/staff/dashboard?from=${from}&to=${to}${project ? `&project=${project}` : ''}`, { keepPrevious: true })

  return (
    <section>
      <PageHead title="Dashboard" intro="Leads that came in during the period, and what happened to them since.">
        <select className="select" aria-label="Project" value={project} onChange={(e) => setProject(e.target.value)}>
          <option value="">Both projects</option>
          {projects.map((p) => (
            <option key={p.id} value={p.id}>
              {p.name}
            </option>
          ))}
        </select>
      </PageHead>

      <div className="segmented period-switch" role="group" aria-label="Period">
        {periods.map((p) => (
          <button key={p.id} aria-pressed={period === p.id} onClick={() => setPeriod(p.id)}>
            {p.label}
          </button>
        ))}
      </div>

      {res.error ? (
        <ErrorState message={res.error} onRetry={res.reload} />
      ) : !res.data ? (
        <Loading label="Loading the dashboard" />
      ) : res.data.totals.leads === 0 ? (
        <div className="table-wrap">
          <EmptyState title="No leads in this period">Pick a longer period, or the other project.</EmptyState>
        </div>
      ) : (
        <DashboardBody d={res.data} busy={res.loading} />
      )}
    </section>
  )
}

function DashboardBody({ d, busy }: { d: Dashboard; busy: boolean }) {
  const t = d.totals
  const costRows = d.bySource
    .filter((m) => m.source !== 'manual')
    .map((m) => ({
      label: sourceLabel[m.source!],
      value: m.costPerVisitInr,
      display: rupees(m.costPerVisitInr),
      note: d.spendNote ? 'Shown for both projects only' : m.spendInr == null ? 'No spend recorded' : 'No visits yet',
    }))
    .sort((a, b) => (b.value ?? -1) - (a.value ?? -1))

  return (
    <div aria-busy={busy} className={busy ? 'is-refreshing' : ''}>
      <div className="kpis">
        <Kpi
          label="Median first reply"
          value={duration(t.medianFirstResponseSeconds)}
          target="Target: under 5 minutes"
          ok={t.medianFirstResponseSeconds != null ? t.medianFirstResponseSeconds < 300 : null}
          detail={`${pct(t.respondedWithin5MinPct)} answered within 5 minutes`}
        />
        <Kpi
          label="Not reached in 48 hours"
          value={pct(t.unreachedIn48hPct)}
          target="Target: under 5%"
          ok={t.unreachedIn48hPct != null ? t.unreachedIn48hPct < 5 : null}
          detail={`${t.unreachedIn48h} of ${t.oldEnoughFor48h} leads old enough to count`}
        />
        <Kpi
          label="Lead to visit"
          value={pct(t.leadToVisitPct)}
          target="Target: 8%"
          ok={t.leadToVisitPct != null ? t.leadToVisitPct >= 8 : null}
          detail={`${t.leadsVisited} visited of ${t.leads} leads · ${t.leadsWithVisitBooked} booked`}
        />
        <Kpi
          label="Show-up rate"
          value={pct(t.showUpRatePct)}
          detail={`${t.visitsDone} came, ${t.noShows} did not`}
        />
        <Kpi
          label="Cost per visit"
          value={rupees(t.costPerVisitInr)}
          detail={t.costPerLeadInr != null ? `${rupees(t.costPerLeadInr)} per lead` : 'Needs spend for every source'}
        />
      </div>

      <div className="dash-grid">
        <div className="panel">
          <div className="panel-title">
            <h2>Cost per visit, by source</h2>
          </div>
          <HorizontalBars rows={costRows} />
          <p className="muted small chart-note">
            {d.spendNote ??
              'Spend divided by buyers who came for a visit. Spend counts every calendar month the period touches.'}
          </p>
        </div>

        <div className="panel">
          <div className="panel-title">
            <h2>Leads per day</h2>
          </div>
          <DailyColumns
            unit="leads"
            points={d.daily.map((p) => ({
              date: p.date,
              value: p.leads,
              note: p.medianFirstResponseSeconds != null ? `median reply ${duration(p.medianFirstResponseSeconds)}` : undefined,
            }))}
          />
          <details className="chart-table">
            <summary>Show as a table</summary>
            <table className="table">
              <thead>
                <tr>
                  <th>Day</th>
                  <th>Leads</th>
                  <th>Median first reply</th>
                </tr>
              </thead>
              <tbody>
                {d.daily.map((p) => (
                  <tr key={p.date}>
                    <td className="num">{p.date}</td>
                    <td className="num">{p.leads}</td>
                    <td className="num">{duration(p.medianFirstResponseSeconds)}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </details>
        </div>
      </div>

      <div className="panel">
        <div className="panel-title">
          <h2>By source</h2>
        </div>
        <div className="table-scroll">
          <table className="table source-table">
            <thead>
              <tr>
                <th>Source</th>
                <th>Leads</th>
                <th>Median first reply</th>
                <th>Not reached in 48 h</th>
                <th>Visited</th>
                <th>Lead to visit</th>
                <th>Show-up</th>
                <th>Spend</th>
                <th>Per lead</th>
                <th>Per visit</th>
              </tr>
            </thead>
            <tbody>
              {d.bySource.map((m) => (
                <SourceRow key={m.source} m={m} label={sourceLabel[m.source!]} />
              ))}
            </tbody>
            <tfoot>
              <SourceRow m={t} label="All sources" />
            </tfoot>
          </table>
        </div>
      </div>
    </div>
  )
}

function SourceRow({ m, label }: { m: Metrics; label: string }) {
  return (
    <tr>
      <th scope="row">{label}</th>
      <td className="num">{m.leads}</td>
      <td className="num">{duration(m.medianFirstResponseSeconds)}</td>
      <td className="num">{pct(m.unreachedIn48hPct)}</td>
      <td className="num">{m.leadsVisited}</td>
      <td className="num">{pct(m.leadToVisitPct)}</td>
      <td className="num">{pct(m.showUpRatePct)}</td>
      <td className="num">{rupees(m.spendInr)}</td>
      <td className="num">{rupees(m.costPerLeadInr)}</td>
      <td className="num">{rupees(m.costPerVisitInr)}</td>
    </tr>
  )
}

function Kpi({
  label,
  value,
  target,
  ok,
  detail,
}: {
  label: string
  value: string
  target?: string
  ok?: boolean | null
  detail?: string
}) {
  return (
    <div className="kpi">
      <p className="kpi-label">{label}</p>
      <p className="kpi-value num">{value}</p>
      {target && (
        <p className="kpi-target">
          {ok != null && <Chip tone={ok ? 'good' : 'warn'}>{ok ? 'On target' : 'Off target'}</Chip>}
          <span className="muted small">{target}</span>
        </p>
      )}
      {detail && <p className="muted small">{detail}</p>}
    </div>
  )
}
