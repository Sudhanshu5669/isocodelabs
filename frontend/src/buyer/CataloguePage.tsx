import { useMemo, useState } from 'react'
import { Link } from 'react-router-dom'
import { EmptyState, ErrorState } from '../components/ui'
import { useApi } from '../lib/useApi'
import { coverFor, lakhs, type Unit } from './catalogue'

// The catalogue: every flat for sale, with filters a buyer actually uses
// (project, size, budget) and a sort. Each card opens the flat's 3D tour.

type Budget = 'any' | 'under80' | '80to100' | 'over100'
const budgets: { id: Budget; label: string; test: (l: number) => boolean }[] = [
  { id: 'any', label: 'Any', test: () => true },
  { id: 'under80', label: 'Under ₹80 L', test: (l) => l < 80 },
  { id: '80to100', label: '₹80 L – ₹1 Cr', test: (l) => l >= 80 && l < 100 },
  { id: 'over100', label: 'Above ₹1 Cr', test: (l) => l >= 100 },
]

export default function CataloguePage() {
  const { data, error, loading, reload } = useApi<{ units: Unit[] }>('/units')
  const [project, setProject] = useState('all')
  const [bhk, setBhk] = useState<'all' | '2' | '3'>('all')
  const [budget, setBudget] = useState<Budget>('any')
  const [sort, setSort] = useState('recommended')

  const units = data?.units ?? []
  const projects = useMemo(() => [...new Map(units.map((u) => [u.projectId, u.projectName])).entries()], [units])
  const priceRange = units.length
    ? `${lakhs(Math.min(...units.map((u) => u.indicativePriceLakh)))} to ${lakhs(Math.max(...units.map((u) => u.indicativePriceLakh)))}`
    : '–'

  const shown = useMemo(() => {
    const test = budgets.find((b) => b.id === budget)!.test
    const list = units.filter(
      (u) => (project === 'all' || u.projectId === project) && (bhk === 'all' || String(u.bhk) === bhk) && test(u.indicativePriceLakh),
    )
    if (sort === 'priceUp') return [...list].sort((a, b) => a.indicativePriceLakh - b.indicativePriceLakh)
    if (sort === 'priceDown') return [...list].sort((a, b) => b.indicativePriceLakh - a.indicativePriceLakh)
    if (sort === 'area') return [...list].sort((a, b) => b.areaSqft - a.areaSqft)
    return list
  }, [units, project, bhk, budget, sort])

  const filtered = project !== 'all' || bhk !== 'all' || budget !== 'any'
  const clear = () => {
    setProject('all')
    setBhk('all')
    setBudget('any')
  }

  return (
    <main className="pub-main">
      <section className="cat-hero">
        <div>
          <p className="eyebrow">{projects.length || 2} live projects in Ahmedabad</p>
          <h1>Walk through your home before you visit it.</h1>
          <p className="cat-lede">
            Browse 2 and 3 BHK homes, step inside each one in 3D, then book a site visit in a time that is really free.
          </p>
        </div>
        <dl className="cat-facts">
          <div>
            <dt>Homes listed</dt>
            <dd>{loading ? '–' : units.length}</dd>
          </div>
          <div>
            <dt>Configurations</dt>
            <dd>2 and 3 BHK</dd>
          </div>
          <div>
            <dt>Indicative prices</dt>
            <dd>{priceRange}</dd>
          </div>
        </dl>
      </section>

      <section aria-label="Homes" className="cat-list">
        <div className="cat-filters">
          <div className="cat-filter-row">
            <ChipGroup
              label="Project"
              value={project}
              onChange={setProject}
              options={[['all', 'All'], ...projects.map(([id, name]) => [id, name] as [string, string])]}
            />
            <ChipGroup label="Size" value={bhk} onChange={(v) => setBhk(v as typeof bhk)} options={[['all', 'All'], ['2', '2 BHK'], ['3', '3 BHK']]} />
          </div>
          <div className="cat-filter-row">
            <ChipGroup label="Budget" value={budget} onChange={(v) => setBudget(v as Budget)} options={budgets.map((b) => [b.id, b.label])} />
            <label className="cat-sort">
              Sort
              <select className="select" value={sort} onChange={(e) => setSort(e.target.value)}>
                <option value="recommended">Recommended</option>
                <option value="priceUp">Price, low to high</option>
                <option value="priceDown">Price, high to low</option>
                <option value="area">Largest first</option>
              </select>
            </label>
          </div>
        </div>

        <div className="cat-count">
          <p aria-live="polite">{loading ? 'Loading homes' : `${shown.length} of ${units.length} homes`}</p>
          {filtered && (
            <button className="btn btn-quiet btn-small" onClick={clear}>
              Clear filters
            </button>
          )}
        </div>

        {error ? (
          <ErrorState message={error} onRetry={reload} />
        ) : loading ? (
          <div className="cat-grid" aria-hidden="true">
            {[0, 1, 2, 3, 4, 5].map((i) => (
              <div key={i} className="skeleton" style={{ height: 380, borderRadius: 20 }} />
            ))}
          </div>
        ) : shown.length === 0 ? (
          <div className="cat-empty">
            <EmptyState title="No homes match those filters">
              Try a wider budget, or tell the assistant what you need and it will find the closest fit.
            </EmptyState>
            <div className="actions" style={{ justifyContent: 'center' }}>
              <button className="btn" onClick={clear}>
                Clear filters
              </button>
              <Link className="btn btn-quiet" to="/chat">
                Ask the assistant
              </Link>
            </div>
          </div>
        ) : (
          <div className="cat-grid">
            {shown.map((u, i) => {
              const cover = coverFor(u, i)
              return (
                <Link key={u.code} to={`/homes/${u.code}`} className="cat-card">
                  <div className="cat-card-img">
                    <img src={cover.src} alt={`${cover.name} in a furnished ${u.bhk} BHK`} loading={i < 3 ? 'eager' : 'lazy'} decoding="async" />
                    <span className="cat-badge">
                      <CubeIcon /> 3D tour
                    </span>
                  </div>
                  <div className="cat-card-body">
                    <p className="cat-card-kicker">
                      {u.projectName} · {u.code}
                    </p>
                    <h2>{u.bhk} BHK residence</h2>
                    <p className="muted small">
                      {u.areaSqft.toLocaleString('en-IN')} sq ft · {u.facing} facing · Floor {u.floor}
                    </p>
                    <div className="cat-card-foot">
                      <div>
                        <p className="cat-price">{lakhs(u.indicativePriceLakh)}</p>
                        <p className="muted small">Indicative price</p>
                      </div>
                      <span className="cat-step">Step inside →</span>
                    </div>
                  </div>
                </Link>
              )
            })}
          </div>
        )}
      </section>
    </main>
  )
}

function ChipGroup({ label, value, onChange, options }: { label: string; value: string; onChange: (v: string) => void; options: [string, string][] }) {
  return (
    <div className="cat-chips" role="group" aria-label={label}>
      <span className="cat-chips-label">{label}</span>
      {options.map(([id, text]) => (
        <button key={id} type="button" className="fchip" aria-pressed={value === id} onClick={() => onChange(id)}>
          {text}
        </button>
      ))}
    </div>
  )
}

export function CubeIcon() {
  return (
    <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinejoin="round" aria-hidden="true">
      <path d="M12 2 3 7v10l9 5 9-5V7l-9-5Z" />
      <path d="m3 7 9 5 9-5M12 12v10" />
    </svg>
  )
}
