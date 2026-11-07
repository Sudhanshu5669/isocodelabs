import { useMemo, useState } from 'react'
import { Link } from 'react-router-dom'
import { EmptyState, ErrorState } from '../components/ui'
import { useApi } from '../lib/useApi'
import { coverFor, floorLabel, lakhs, type Unit } from './catalogue'

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
  const minArea = units.length ? Math.min(...units.map((u) => u.areaSqft)).toLocaleString('en-IN') + '+' : '–'
  const fromPrice = units.length ? lakhs(Math.min(...units.map((u) => u.indicativePriceLakh))) : '–'
  const reraNumbers = [...new Set(units.map((u) => u.reraNumber))]

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
    <main className="cat">
      <section className="cat-hero">
        <img className="cat-hero-img" src="/images/rooms/living.jpg" alt="" fetchPriority="high" />
        <div className="cat-wrap cat-hero-inner">
          <div className="cat-hero-copy">
            <p className="cat-hero-eyebrow">Ahmedabad · 2 &amp; 3 BHK residences</p>
            <h1>
              Homes worth
              <br />a closer look.
            </h1>
            <p className="cat-lede">
              Walk through every flat in 3D, ask anything in English, Hindi or Gujarati, and book a site visit in a time
              that is really free.
            </p>
            <div className="actions">
              <a href="#homes" className="btn btn-large">
                See the homes
              </a>
              <Link to="/chat" className="btn btn-large btn-outline">
                Ask a question
              </Link>
            </div>
          </div>
          <aside className="cat-glance" aria-label="At a glance">
            <p className="eyebrow">At a glance</p>
            <dl>
              <div>
                <dt>projects</dt>
                <dd>{loading ? '–' : projects.length}</dd>
              </div>
              <div>
                <dt>homes open</dt>
                <dd>{loading ? '–' : units.length}</dd>
              </div>
              <div>
                <dt>sq ft</dt>
                <dd>{minArea}</dd>
              </div>
              <div>
                <dt>indicative, from</dt>
                <dd>{fromPrice}</dd>
              </div>
            </dl>
            {reraNumbers.length > 0 && <p className="cat-glance-rera">{reraNumbers.join(' · ')}</p>}
          </aside>
        </div>
      </section>

      <section className="cat-wrap cat-statement">
        <p className="cat-num">01/ 03</p>
        <h2>
          Ask at midnight. Get a straight answer. <span>Meet a real person when it matters.</span>
        </h2>
      </section>

      <section id="homes" aria-label="Homes" className="cat-wrap cat-list">
        <div className="cat-list-head">
          <div>
            <p className="cat-num">02/ 03 · The homes</p>
            <p aria-live="polite">
              {loading ? 'Loading homes' : `${shown.length} of ${units.length} homes`} · prices are indicative, confirmed by
              the sales team
            </p>
          </div>
          {filtered && (
            <button className="btn btn-quiet btn-small" onClick={clear}>
              Clear filters
            </button>
          )}
        </div>
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
                    <span className="cat-code">
                      {u.code} · {u.projectName}
                    </span>
                    <span className="cat-badge">
                      <CubeIcon /> 3D tour
                    </span>
                  </div>
                  <div className="cat-card-title">
                    <h3>
                      {u.bhk} BHK · Tower {u.tower}
                    </h3>
                    <span className="cat-price">{lakhs(u.indicativePriceLakh)}</span>
                  </div>
                  <div className="cat-card-foot">
                    <span>
                      {u.areaSqft.toLocaleString('en-IN')} sq ft · {floorLabel(u.floor)} · {u.facing}
                    </span>
                    <span className="cat-step">
                      View <span aria-hidden="true">→</span>
                    </span>
                  </div>
                </Link>
              )
            })}
          </div>
        )}
      </section>

      <section id="how" className="cat-how">
        <div className="cat-wrap">
          <div className="cat-how-head">
            <h2>
              From question
              <br />
              to front door.
            </h2>
            <p className="cat-num">03/ 03 · How visits work</p>
          </div>
          <ol className="cat-steps">
            <li>
              <span className="cat-num">01</span>
              <h3>Ask anything</h3>
              <p>The assistant answers from the approved fact sheet only. If it does not know, it says so and a person follows up.</p>
            </li>
            <li>
              <span className="cat-num">02</span>
              <h3>Pick a free time</h3>
              <p>You only see times that are still open, so a visit is never double booked. Change or cancel from your visit link.</p>
            </li>
            <li>
              <span className="cat-num">03</span>
              <h3>Meet your advisor</h3>
              <p>A named salesperson meets you on site. You get a reminder a day before and two hours before.</p>
            </li>
          </ol>
        </div>
      </section>

      <section className="cat-cta">
        <div className="cat-wrap">
          <h2>
            Tell us what
            <br />
            you need.
          </h2>
          <Link to="/chat" className="btn btn-large">
            Start a chat <span aria-hidden="true">→</span>
          </Link>
        </div>
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
