import type { Plan } from './plans'

// A small floor plan drawn from the same plan data as the 3D model, so the two
// always agree. The current room is filled in; tapping a room opens it.
export default function FloorPlan({ plan, active, onPick }: { plan: Plan; active: string | null; onPick: (id: string) => void }) {
  const minZ = Math.min(...plan.rooms.map((r) => r.z0))
  const pad = 0.3
  const vb = `${-pad} ${minZ - pad} ${plan.width + pad * 2} ${plan.depth - minZ + pad * 2}`
  return (
    <svg className="plan-svg" viewBox={vb} aria-hidden="true">
      {plan.rooms.map((r) => (
        <rect
          key={r.id}
          x={r.x0}
          y={r.z0}
          width={r.x1 - r.x0}
          height={r.z1 - r.z0}
          className={`plan-room ${r.id === active ? 'is-active' : ''} ${r.floor === 'deck' ? 'is-outdoor' : ''}`}
          onClick={() => onPick(r.id)}
        >
          <title>{r.name}</title>
        </rect>
      ))}
    </svg>
  )
}
