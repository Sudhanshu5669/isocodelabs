// The two floor plans, in metres. x runs west to east, z runs north to south,
// and the balcony is on the north side (z < 0). Each plan lists:
//   rooms     rectangles with a floor finish and a viewpoint for the walk-in view
//   walls     straight wall runs with their doors and windows
//   furniture what stands where; rot is degrees, turning the piece's front
//             (which faces +z) toward +x at 90, -z at 180, -x at -90
//
// Plans are illustrative (labelled so on the page): the brief gives sizes per
// configuration, not drawings. Areas match the fact sheet's ranges.

export type Layout = '2bhk' | '3bhk'
export type Floor = 'wood' | 'stone' | 'deck'

export type Room = {
  id: string
  name: string
  x0: number
  z0: number
  x1: number
  z1: number
  floor: Floor
  /** Where you stand and what you face in the walk-in view. */
  eye: [number, number]
  look: [number, number]
  /** Small spaces (bathrooms, hall) are reachable but not listed first. */
  minor?: boolean
}

export type OpeningKind = 'door' | 'window' | 'slider' | 'open' | 'high'
export type Opening = { from: number; to: number; kind: OpeningKind }

/** A wall along x (at z = at) or along z (at x = at), from `from` to `to`. */
export type Wall = {
  along: 'x' | 'z'
  at: number
  from: number
  to: number
  exterior?: boolean
  rail?: boolean
  openings?: Opening[]
}

export type Item = { t: string; x: number; z: number; r?: number; s?: number[] }

export type Plan = {
  layout: Layout
  label: string
  width: number
  depth: number
  rooms: Room[]
  walls: Wall[]
  furniture: Item[]
}

// ---- shared pieces: the living room, dining and balcony are the same in both plans

const livingRoom: Room = {
  id: 'living', name: 'Living and dining', x0: 0, z0: 0, x1: 5.6, z1: 5, floor: 'wood', eye: [5.1, 4.6], look: [1.4, 1.6],
}
const balcony: Room = {
  id: 'balcony', name: 'Balcony', x0: 0, z0: -1.6, x1: 5.6, z1: 0, floor: 'deck', eye: [5.2, -0.25], look: [0.8, -1.1],
}
const bathroom: Room = {
  id: 'bath', name: 'Bathroom', x0: 3.6, z0: 5, x1: 5.6, z1: 7.2, floor: 'stone', eye: [5.25, 5.35], look: [4, 6.9], minor: true,
}

const balconyWalls: Wall[] = [
  { along: 'x', at: -1.6, from: 0, to: 5.6, rail: true },
  { along: 'z', at: 0, from: -1.6, to: 0, rail: true },
  { along: 'z', at: 5.6, from: -1.6, to: 0, rail: true },
]

// Art hangs only on north and west walls: the south and east walls are the
// ones cut away towards the viewer in the diorama.
//
// The living room follows the reference look: a slatted wood TV wall with a warm
// LED line, a cream sectional with a chaise, a round walnut table, an olive tree.
const livingFurniture: Item[] = [
  { t: 'rug', x: 1.95, z: 2.45, s: [2.7, 2.3] },
  { t: 'slatWall', x: 0.13, z: 2.45, r: 90, s: [3.4] },
  { t: 'tvUnit', x: 0.36, z: 2.45, r: 90, s: [2.0] },
  { t: 'sectional', x: 3.15, z: 2.4, r: -90, s: [2.5] },
  { t: 'coffeeTable', x: 1.75, z: 2.05 },
  { t: 'arcLamp', x: 3.3, z: 4.3, r: -90 },
  { t: 'plant', x: 0.5, z: 0.55, s: [1.7] },
  { t: 'shelf', x: 0.3, z: 4.45, r: 90, s: [0.9] },
  { t: 'plant', x: 2.95, z: 0.45, s: [0.8] },
  { t: 'diningTable', x: 4.6, z: 2.15, r: 90, s: [1.5, 0.9] },
  { t: 'chair', x: 4.05, z: 1.7, r: 90 },
  { t: 'chair', x: 4.05, z: 2.6, r: 90 },
  { t: 'chair', x: 5.15, z: 1.7, r: -90 },
  { t: 'chair', x: 5.15, z: 2.6, r: -90 },
  { t: 'pendant', x: 4.6, z: 1.85 },
  { t: 'pendant', x: 4.6, z: 2.45 },
  { t: 'sideboard', x: 4.6, z: 4.68, r: 180, s: [1.4] },
]

const balconyFurniture: Item[] = [
  { t: 'loungeChair', x: 1.3, z: -0.8, r: 90 },
  { t: 'loungeChair', x: 3.1, z: -0.8, r: -90 },
  { t: 'sideTable', x: 2.2, z: -0.8 },
  { t: 'planter', x: 4.6, z: -1.3, s: [1.5] },
  { t: 'plant', x: 0.35, z: -1.25, s: [1.1] },
]

const bathroomFurniture: Item[] = [
  { t: 'vanity', x: 5.3, z: 5.75, r: -90 },
  { t: 'shower', x: 5.13, z: 6.73 },
  { t: 'toilet', x: 3.98, z: 6.6, r: 90 },
]

// ---- 2 BHK: about 90 m² inside the walls (fact sheet: 1,085 to 1,150 sq ft)

const twoBhk: Plan = {
  layout: '2bhk',
  label: '2 BHK',
  width: 10,
  depth: 9,
  rooms: [
    livingRoom,
    { id: 'kitchen', name: 'Kitchen', x0: 5.6, z0: 0, x1: 10, z1: 3.2, floor: 'stone', eye: [6.1, 2.9], look: [9.2, 0.6] },
    { id: 'master', name: 'Main bedroom', x0: 5.6, z0: 5, x1: 10, z1: 9, floor: 'wood', eye: [6.3, 8.6], look: [8.7, 5.4] },
    { id: 'bed2', name: 'Bedroom 2', x0: 0, z0: 5, x1: 3.6, z1: 9, floor: 'wood', eye: [3.1, 8.6], look: [0.9, 5.5] },
    balcony,
    { id: 'foyer', name: 'Entrance', x0: 5.6, z0: 3.2, x1: 10, z1: 5, floor: 'stone', eye: [9.5, 4.1], look: [5.8, 4.1], minor: true },
    bathroom,
    { id: 'ensuite', name: 'Main bathroom', x0: 3.6, z0: 7.2, x1: 5.6, z1: 9, floor: 'stone', eye: [5.25, 7.6], look: [3.9, 8.7], minor: true },
  ],
  walls: [
    { along: 'x', at: 0, from: 0, to: 5.6, exterior: true, openings: [{ from: 0.6, to: 3.4, kind: 'slider' }] },
    { along: 'x', at: 0, from: 5.6, to: 10, exterior: true, openings: [{ from: 7.2, to: 8.8, kind: 'window' }] },
    { along: 'z', at: 10, from: 0, to: 9, exterior: true, openings: [{ from: 3.6, to: 4.6, kind: 'door' }] },
    {
      along: 'x', at: 9, from: 0, to: 10, exterior: true,
      openings: [
        { from: 0.4, to: 1.3, kind: 'window' },
        { from: 4.3, to: 5.0, kind: 'high' },
        { from: 6.9, to: 8.9, kind: 'window' },
      ],
    },
    {
      along: 'z', at: 0, from: 0, to: 9, exterior: true,
      openings: [{ from: 1.2, to: 3.2, kind: 'high' }],
    },
    { along: 'x', at: 5, from: 0, to: 5.6, openings: [{ from: 2.4, to: 3.3, kind: 'door' }, { from: 4.25, to: 5.05, kind: 'door' }] },
    { along: 'x', at: 5, from: 5.6, to: 10, openings: [{ from: 6.3, to: 7.2, kind: 'door' }] },
    { along: 'z', at: 3.6, from: 5, to: 9 },
    { along: 'z', at: 5.6, from: 5, to: 9, openings: [{ from: 7.7, to: 8.5, kind: 'door' }] },
    { along: 'x', at: 7.2, from: 3.6, to: 5.6 },
    { along: 'x', at: 3.2, from: 5.6, to: 10, openings: [{ from: 6.0, to: 7.0, kind: 'open' }] },
    { along: 'z', at: 5.6, from: 0, to: 3.2, openings: [{ from: 0.5, to: 2.7, kind: 'open' }] },
    ...balconyWalls,
  ],
  furniture: [
    ...livingFurniture,
    ...balconyFurniture,
    ...bathroomFurniture,
    // kitchen
    { t: 'fridge', x: 6.05, z: 0.46 },
    { t: 'kitchenRun', x: 8.25, z: 0.41, s: [3.3, 1, 0, 0.0] },
    { t: 'kitchenRun', x: 9.59, z: 1.8, r: -90, s: [2.2, 0, 1, 1] },
    { t: 'stool', x: 6.4, z: 2.6 },
    { t: 'plant', x: 9.6, z: 2.95, s: [0.6] },
    // entrance
    { t: 'sideboard', x: 8.3, z: 4.7, r: 180, s: [1.4] },
    { t: 'plant', x: 9.6, z: 3.5, s: [1.2] },
    // main bedroom: the headboard is on a back wall, so the diorama shows it
    { t: 'rug', x: 8.4, z: 6.7, s: [2.6, 2.3] },
    { t: 'bed', x: 8.4, z: 6.13, s: [1.8, 2.1] },
    { t: 'nightstand', x: 7.2, z: 5.27 },
    { t: 'nightstand', x: 9.6, z: 5.27 },
    { t: 'art', x: 8.4, z: 5.08, s: [1.3, 0.6, 3] },
    { t: 'wardrobe', x: 5.98, z: 6.35, r: 90, s: [1.6] },
    { t: 'armchair', x: 6.5, z: 8.4, r: 135 },
    { t: 'arcLamp', x: 5.95, z: 8.75 },
    // bedroom 2: bed against the north wall, wardrobe on the west wall
    { t: 'rug', x: 1.35, z: 6.8, s: [1.8, 2.2] },
    { t: 'bed', x: 1.05, z: 6.08, s: [1.5, 2.0] },
    { t: 'nightstand', x: 2.1, z: 5.27 },
    { t: 'art', x: 1.05, z: 5.08, s: [0.9, 0.6, 2] },
    { t: 'wardrobe', x: 0.36, z: 7.95, r: 90, s: [1.8] },
    { t: 'desk', x: 3.25, z: 6.4, r: -90 },
    { t: 'chair', x: 2.7, z: 6.4, r: 90 },
    // main bathroom
    { t: 'vanity', x: 4.6, z: 7.5 },
    { t: 'toilet', x: 3.98, z: 8.6, r: 90 },
    { t: 'shower', x: 5.13, z: 8.53, s: [0.85] },
  ],
}

// ---- 3 BHK: about 126 m² inside the walls (fact sheet: 1,590 to 1,710 sq ft)

const threeBhk: Plan = {
  layout: '3bhk',
  label: '3 BHK',
  width: 13.4,
  depth: 9.4,
  rooms: [
    livingRoom,
    { id: 'kitchen', name: 'Kitchen', x0: 5.6, z0: 0, x1: 9.6, z1: 3.2, floor: 'stone', eye: [6.1, 2.9], look: [9, 0.6] },
    { id: 'master', name: 'Main bedroom', x0: 7.6, z0: 5, x1: 13.4, z1: 9.4, floor: 'wood', eye: [8.4, 8.9], look: [11.8, 5.5] },
    { id: 'bed2', name: 'Bedroom 2', x0: 0, z0: 5, x1: 3.6, z1: 9.4, floor: 'wood', eye: [3.1, 8.9], look: [0.9, 5.5] },
    { id: 'bed3', name: 'Bedroom 3', x0: 9.6, z0: 0, x1: 13.4, z1: 3.2, floor: 'wood', eye: [11.6, 2.9], look: [10.4, 0.5] },
    balcony,
    { id: 'hall', name: 'Entrance hall', x0: 5.6, z0: 3.2, x1: 13.4, z1: 5, floor: 'stone', eye: [13, 4.1], look: [5.8, 4.1], minor: true },
    bathroom,
    { id: 'bath2', name: 'Second bathroom', x0: 5.6, z0: 5, x1: 7.6, z1: 7.2, floor: 'stone', eye: [7.25, 5.35], look: [6, 6.9], minor: true },
    { id: 'ensuite', name: 'Main bathroom', x0: 5.6, z0: 7.2, x1: 7.6, z1: 9.4, floor: 'stone', eye: [7.25, 7.6], look: [5.9, 9.1], minor: true },
    { id: 'dressing', name: 'Dressing room', x0: 3.6, z0: 7.2, x1: 5.6, z1: 9.4, floor: 'wood', eye: [3.95, 7.55], look: [5.3, 9], minor: true },
  ],
  walls: [
    { along: 'x', at: 0, from: 0, to: 5.6, exterior: true, openings: [{ from: 0.6, to: 3.4, kind: 'slider' }] },
    { along: 'x', at: 0, from: 5.6, to: 9.6, exterior: true, openings: [{ from: 7.2, to: 8.8, kind: 'window' }] },
    { along: 'x', at: 0, from: 9.6, to: 13.4, exterior: true, openings: [{ from: 11.6, to: 13.0, kind: 'window' }] },
    {
      along: 'z', at: 13.4, from: 0, to: 9.4, exterior: true,
      openings: [{ from: 0.6, to: 2.2, kind: 'window' }, { from: 3.6, to: 4.6, kind: 'door' }],
    },
    {
      along: 'x', at: 9.4, from: 0, to: 13.4, exterior: true,
      openings: [
        { from: 0.4, to: 1.3, kind: 'window' },
        { from: 4.2, to: 5.0, kind: 'high' },
        { from: 6.2, to: 7.0, kind: 'high' },
        { from: 9.0, to: 11.4, kind: 'window' },
      ],
    },
    {
      along: 'z', at: 0, from: 0, to: 9.4, exterior: true,
      openings: [{ from: 1.2, to: 3.2, kind: 'high' }],
    },
    { along: 'x', at: 5, from: 0, to: 5.6, openings: [{ from: 2.4, to: 3.3, kind: 'door' }, { from: 4.25, to: 5.05, kind: 'door' }] },
    { along: 'x', at: 5, from: 5.6, to: 13.4, openings: [{ from: 6.1, to: 6.9, kind: 'door' }, { from: 8.6, to: 9.5, kind: 'door' }] },
    { along: 'z', at: 3.6, from: 5, to: 9.4, openings: [{ from: 7.55, to: 8.35, kind: 'door' }] },
    { along: 'z', at: 5.6, from: 5, to: 9.4 },
    { along: 'x', at: 7.2, from: 3.6, to: 7.6 },
    { along: 'z', at: 7.6, from: 5, to: 9.4, openings: [{ from: 7.8, to: 8.6, kind: 'door' }] },
    { along: 'x', at: 3.2, from: 5.6, to: 9.6, openings: [{ from: 6.0, to: 7.0, kind: 'open' }] },
    { along: 'x', at: 3.2, from: 9.6, to: 13.4, openings: [{ from: 10.2, to: 11.1, kind: 'door' }] },
    { along: 'z', at: 9.6, from: 0, to: 3.2 },
    { along: 'z', at: 5.6, from: 0, to: 3.2, openings: [{ from: 0.5, to: 2.7, kind: 'open' }] },
    ...balconyWalls,
  ],
  furniture: [
    ...livingFurniture,
    ...balconyFurniture,
    ...bathroomFurniture,
    // kitchen
    { t: 'fridge', x: 6.05, z: 0.46 },
    { t: 'kitchenRun', x: 8.0, z: 0.41, s: [3.0, 1, 0, 0] },
    { t: 'kitchenRun', x: 9.22, z: 1.8, r: -90, s: [2.2, 0, 1, 1] },
    { t: 'stool', x: 6.4, z: 2.6 },
    // bedroom 3
    { t: 'rug', x: 10.9, z: 1.5, s: [2.2, 1.8] },
    { t: 'bed', x: 10.68, z: 1.5, r: 90, s: [1.4, 2.0] },
    { t: 'nightstand', x: 9.93, z: 0.4, r: 90 },
    { t: 'nightstand', x: 9.93, z: 2.6, r: 90 },
    { t: 'desk', x: 13.0, z: 1.4, r: -90 },
    { t: 'chair', x: 12.45, z: 1.4, r: 90 },
    { t: 'wardrobe', x: 12.35, z: 2.82, r: 180, s: [1.8] },
    // entrance hall
    { t: 'sideboard', x: 11.6, z: 4.7, r: 180, s: [1.4] },
    { t: 'art', x: 8.2, z: 3.28, s: [1.0, 0.7, 1] },
    { t: 'plant', x: 13.05, z: 3.5, s: [1.2] },
    // main bedroom: the headboard is on a back wall, so the diorama shows it
    { t: 'rug', x: 11.5, z: 6.75, s: [2.8, 2.4] },
    { t: 'bed', x: 11.5, z: 6.13, s: [1.8, 2.1] },
    { t: 'nightstand', x: 10.3, z: 5.27 },
    { t: 'nightstand', x: 12.7, z: 5.27 },
    { t: 'art', x: 11.5, z: 5.08, s: [1.4, 0.65, 3] },
    { t: 'wardrobe', x: 7.98, z: 6.35, r: 90, s: [1.8] },
    { t: 'armchair', x: 9.1, z: 8.6, r: 150 },
    { t: 'arcLamp', x: 8.1, z: 9.0 },
    { t: 'plant', x: 13.0, z: 8.95, s: [1.3] },
    // bedroom 2: bed against the north wall, wardrobe on the west wall
    { t: 'rug', x: 1.35, z: 6.8, s: [1.8, 2.2] },
    { t: 'bed', x: 1.05, z: 6.08, s: [1.5, 2.0] },
    { t: 'nightstand', x: 2.1, z: 5.27 },
    { t: 'art', x: 1.05, z: 5.08, s: [0.9, 0.6, 2] },
    { t: 'wardrobe', x: 0.36, z: 8.3, r: 90, s: [1.8] },
    { t: 'desk', x: 3.25, z: 6.3, r: -90 },
    { t: 'chair', x: 2.7, z: 6.3, r: 90 },
    // dressing room
    { t: 'wardrobe', x: 5.22, z: 8.3, r: -90, s: [2.0] },
    { t: 'art', x: 4.3, z: 7.29, s: [0.5, 1.3, 4] },
    { t: 'stool', x: 4.3, z: 8.6 },
    // second bathroom
    { t: 'vanity', x: 5.95, z: 5.8, r: 90 },
    { t: 'shower', x: 6.13, z: 6.73 },
    { t: 'toilet', x: 7.25, z: 6.7, r: -90 },
    // main bathroom
    { t: 'vanity', x: 6.6, z: 7.5 },
    { t: 'toilet', x: 5.95, z: 8.95, r: 90 },
    { t: 'shower', x: 7.13, z: 8.93, s: [0.85] },
  ],
}

export const plans: Record<Layout, Plan> = { '2bhk': twoBhk, '3bhk': threeBhk }

/** Floor area of a room in square feet, for labels. */
export function roomArea(r: Room): string {
  const m2 = (r.x1 - r.x0) * (r.z1 - r.z0)
  const ft = (m: number) => Math.round(m * 3.281)
  return `${ft(r.x1 - r.x0)}′ × ${ft(r.z1 - r.z0)}′ · ${Math.round(m2 * 10.764)} sq ft`
}
