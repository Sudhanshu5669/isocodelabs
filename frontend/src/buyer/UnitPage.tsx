import { lazy, Suspense, useMemo, useState } from 'react'
import { Link, useNavigate, useParams } from 'react-router-dom'
import type { FreeSlot } from '../api/types'
import SlotPicker from '../components/SlotPicker'
import { ErrorState, Loading } from '../components/ui'
import { isoDay, addDays } from '../lib/format'
import { useApi } from '../lib/useApi'
import FloorPlan from '../tour/FloorPlan'
import { planFor, roomArea } from '../tour/plans'
import { describeTheme, flatTheme } from '../tour/themes'
import type { TourView } from '../tour/TourViewer'
import { CubeIcon } from './CataloguePage'
import { bathrooms, floorLabel, lakhs, photosFor, showFallback, type UnitDetail } from './catalogue'
import './unit.css'

// The 3D code (and the 3D library) is downloaded only when this page opens,
// and in parallel with the flat's details. The catalogue never pays for it.
const TourViewer = lazy(() => import('../tour/TourViewer'))

export default function UnitPage() {
  const { code = '' } = useParams()
  const navigate = useNavigate()
  const unit = useApi<UnitDetail>(`/units/${encodeURIComponent(code)}`)
  const today = isoDay(new Date())
  const slots = useApi<{ slots: FreeSlot[] }>(
    unit.data ? `/slots?project=${unit.data.projectId}&from=${today}&to=${addDays(today, 13)}` : null,
  )

  const [view, setView] = useState<TourView>({ mode: 'flat', roomId: null })
  const [hover, setHover] = useState<string | null>(null)
  const [tourFailed, setTourFailed] = useState(false)
  // The flat's own look for its 3D model (palette, light, view, mirroring).
  // Memoised so the 3D scene is built once, not on every render.
  const theme = useMemo(() => (unit.data ? flatTheme(unit.data) : null), [unit.data])

  if (unit.error) {
    return (
      <main className="pub-main">
        <ErrorState message={unit.error} onRetry={unit.reload} />
        <p style={{ textAlign: 'center' }}>
          <Link to="/">See all homes</Link>
        </p>
      </main>
    )
  }
  if (!unit.data) {
    return (
      <main className="pub-main">
        <Loading label="Loading the home" />
      </main>
    )
  }

  const u = unit.data
  // No layout means the flat has no 3D model yet: the page shows its photos instead.
  const plan = u.layout && theme ? planFor(u.layout, theme) : null
  const room = plan && view.roomId ? plan.rooms.find((r) => r.id === view.roomId) : undefined
  const hovered = plan && hover ? plan.rooms.find((r) => r.id === hover) : undefined
  const openRoom = (id: string) => setView((v) => ({ mode: v.mode === 'walk' ? 'walk' : 'room', roomId: id }))
  const facts = u.facts.filter((f) => f.topic !== 'price_range')
  const gallery = photosFor(u)

  return (
    <main className="pub-main unit">
      {plan && theme ? (
      <section className="unit-stage" aria-label="3D tour">
        <Link to="/" className="unit-back">
          ← All homes
        </Link>
        <div className="tour">
          {tourFailed ? (
            <div className="tour-fallback">
              <img src={gallery[0].src} alt="Furnished living room" onError={(e) => showFallback(e, gallery[0].fallback)} />
              <p>This device cannot show the 3D model, so here is a photo of a furnished home instead.</p>
            </div>
          ) : (
            <Suspense fallback={<div className="tour-loading">Preparing the 3D model</div>}>
              <TourViewer plan={plan} theme={theme} view={view} onPick={openRoom} onHover={setHover} onFail={() => setTourFailed(true)} />
            </Suspense>
          )}

          <span className="tour-badge">
            <CubeIcon /> Furnished 3D model
          </span>

          {!tourFailed && (
            <div className="tour-plan">
              <FloorPlan plan={plan} active={view.roomId} onPick={openRoom} />
            </div>
          )}

          <div className="tour-caption" aria-live="polite">
            {room ? (
              <>
                <p className="tour-room">{room.name}</p>
                <p className="tour-size">{roomArea(room)}</p>
              </>
            ) : (
              <>
                <p className="tour-room">{plan.label} · whole home</p>
                <p className="tour-size">{hovered ? `Tap to open the ${hovered.name.toLowerCase()}` : 'Drag to turn it. Tap a room to open it.'}</p>
              </>
            )}
          </div>

          {!tourFailed && (
            <div className="tour-bar">
              <div className="segmented" role="group" aria-label="View">
                <button aria-pressed={view.mode === 'flat'} onClick={() => setView({ mode: 'flat', roomId: null })}>
                  Whole home
                </button>
                <button aria-pressed={view.mode === 'room'} onClick={() => setView({ mode: 'room', roomId: view.roomId ?? 'living' })}>
                  One room
                </button>
                <button aria-pressed={view.mode === 'walk'} onClick={() => setView({ mode: 'walk', roomId: view.roomId ?? 'living' })}>
                  Walk in
                </button>
              </div>
              <div className="tour-rooms" role="group" aria-label="Rooms">
                {plan.rooms.map((r) => (
                  <button key={r.id} className="fchip fchip-glass" aria-pressed={view.roomId === r.id} onClick={() => openRoom(r.id)}>
                    {r.name}
                  </button>
                ))}
              </div>
            </div>
          )}
        </div>
        <p className="muted small unit-note">
          <span className="unit-mood">{describeTheme(theme)}</span>
          {view.mode === 'walk' ? 'Drag to look around. ' : ''}The 3D model and layout are indicative; furniture and finishes may
          differ from the delivered home.
        </p>
      </section>
      ) : (
        <section className="unit-stage" aria-label="Photos of this home">
          <Link to="/" className="unit-back">
            ← All homes
          </Link>
          <div className="tour">
            <div className="tour-fallback">
              <img src={gallery[0].src} alt={`Living room of ${u.code}, furnished`} onError={(e) => showFallback(e, gallery[0].fallback)} />
            </div>
            <span className="tour-badge">Photos · 3D model coming soon</span>
          </div>
          <p className="muted small unit-note">
            The 3D model of this home is not ready yet. The photos below show it furnished, and on a site visit our team walks
            you through the real flat.
          </p>
        </section>
      )}

      <section className="unit-info">
        <div>
          <p className="eyebrow">
            {u.projectName} · Tower {u.tower} · {floorLabel(u.floor)}
          </p>
          <h1>
            {u.bhk} BHK residence {u.code}
          </h1>
          <div className="unit-price">
            <p>{lakhs(u.indicativePriceLakh)}</p>
            <p className="muted small">Indicative. The final price is confirmed by our sales team.</p>
          </div>
          <dl className="unit-facts">
            <div>
              <dt>Area</dt>
              <dd>{u.areaSqft.toLocaleString('en-IN')} sq ft</dd>
            </div>
            <div>
              <dt>Bedrooms</dt>
              <dd>{u.bhk}</dd>
            </div>
            <div>
              <dt>Bathrooms</dt>
              <dd>{bathrooms(u)}</dd>
            </div>
            <div>
              <dt>Facing</dt>
              <dd>{u.facing}</dd>
            </div>
          </dl>
        </div>

        <div>
          <div className="unit-sub">
            <h2>Inside a finished home</h2>
            <p className="muted small">Scroll sideways</p>
          </div>
          <div className="unit-rail" tabIndex={0} role="region" aria-label="Photos">
            {gallery.map((p) => (
              <figure key={p.src}>
                <img
                  src={p.src}
                  alt={`${p.name} of ${u.code}, furnished`}
                  loading="lazy"
                  decoding="async"
                  onError={(e) => showFallback(e, p.fallback)}
                />
                <figcaption>{p.name}</figcaption>
              </figure>
            ))}
          </div>
        </div>

        <div>
          <h2>From the fact sheet</h2>
          <dl className="unit-sheet">
            {facts.map((f) => (
              <div key={f.topic}>
                <dt>{f.label}</dt>
                <dd>{f.value}</dd>
              </div>
            ))}
            <div>
              <dt>RERA registration</dt>
              <dd>{u.reraNumber}</dd>
            </div>
          </dl>
          <p className="muted small" style={{ marginTop: 12 }}>
            Our assistant answers from this same sheet. Anything not listed here, it passes to a person to confirm.
          </p>
        </div>

        <div className="unit-book panel">
          <h2>Book a site visit</h2>
          <p className="muted" style={{ marginTop: 6 }}>
            Pick a time to see {u.projectName}. A slot holds one visit, and it is yours once you confirm in the chat.
          </p>
          <div style={{ marginTop: 20 }}>
            {slots.loading ? (
              <Loading label="Loading free times" />
            ) : slots.error ? (
              <p className="alert alert-error">{slots.error}</p>
            ) : (slots.data?.slots.length ?? 0) === 0 ? (
              <p className="muted">No free times in the next two weeks. Ask the assistant and a salesperson will arrange one.</p>
            ) : (
              <SlotPicker
                slots={slots.data!.slots.slice(0, 8)}
                onPick={(s) => navigate(`/chat?project=${u.projectId}&unit=${u.code}&slot=${s.id}`)}
              />
            )}
          </div>
          <p style={{ marginTop: 20 }}>
            <Link to={`/chat?project=${u.projectId}&unit=${u.code}`} className="btn btn-quiet">
              Ask a question first
            </Link>
          </p>
          <p className="muted small" style={{ marginTop: 16 }}>
            With your consent we send a reminder 1 day and 2 hours before. Reply STOP at any time to stop messages.
          </p>
        </div>
      </section>
    </main>
  )
}
