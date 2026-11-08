// Each flat's 3D model is dressed to match its own photos, so the tour and the
// gallery show the same home:
//
//   palette   upholstery, cushions, throws, kitchen cabinets, rugs, wall tint
//             and art, taken from the flat's photo brief
//   style     the project's finish: Project 1 is warm Indian contemporary
//             (terrazzo floors, light oak); Project 2 is calm modern (large
//             porcelain tiles, walnut)
//   light     the time of day its windows get sun, from the way it faces:
//             east = morning, west = sunset, south = midday, north = soft daylight
//   view      what you see out of the windows, from its floor: garden (low),
//             rooftops (middle) or the city skyline (high)
//   mirrored  Towers B and D are mirror images of Towers A and C, as twin
//             towers often are, so their plans are flipped east to west
//
// A flat that is not listed here (one added later) still gets a fitting light,
// view and style from its facing, floor and project; only the palette is generic.

export type LightMood = 'morning' | 'evening' | 'midday' | 'soft'
export type View = 'garden' | 'rooftops' | 'skyline'
export type Style = 'warm' | 'modern'

export type Palette = {
  sofa: string
  sofaShade: string // seat backs and the darker side of the upholstery
  cushionA: string
  cushionB: string
  throw: string
  chair: string // the flat's signature armchair and the balcony seats
  headboard: string
  cabinet: string
  wall: string
  rug: string
  rugBorder: string
  art: string
}

export type FlatTheme = {
  palette: Palette
  style: Style
  light: LightMood
  view: View
  mirrored: boolean
  /** Many plants (a flat styled as a green home). */
  lush: boolean
}

type UnitLike = { code: string; projectName: string; tower: string; floor: number; facing: string }

const p = (
  sofa: string, sofaShade: string, cushionA: string, cushionB: string, throwColor: string, chair: string,
  headboard: string, cabinet: string, wall: string, rug: string, rugBorder: string, art: string,
): Palette => ({ sofa, sofaShade, cushionA, cushionB, throw: throwColor, chair, headboard, cabinet, wall, rug, rugBorder, art })

/** The palettes from each flat's photo brief (see the images in /images/flats). */
const palettes: Record<string, Palette & { lush?: boolean }> = {
  // sage green and natural oak, a cane armchair and a block-printed throw
  'A-302': p('#9fae8a', '#8d9c79', '#7f8f6a', '#ebe4d2', '#c9cdb5', '#d9c7a3', '#b08d63', '#a9b79a', '#e6e8da', '#d9cbb0', '#bfae8e', '#7f8f6a'),
  // dusty blue and pale ash, indigo dabu cushions
  'B-402': p('#7d93ad', '#6d829c', '#3c4f78', '#e8e2d6', '#9fb0c4', '#e5dccb', '#b08d63', '#8fa1ad', '#dfe4ea', '#d8cdb8', '#7d93ad', '#3c4f78'),
  // charcoal grey and cognac leather
  'B-1006': p('#4a4a4c', '#3d3d40', '#a0603a', '#e8e2d6', '#d9cfbf', '#9a5a35', '#4a4a4c', '#4d4f52', '#ece8e1', '#cfc6b6', '#5a5a5c', '#2b2b2b'),
  // terracotta and cream, a hand-knotted rug
  'A-904': p('#ede4d3', '#ddd2bd', '#c06a45', '#d99a72', '#c06a45', '#e9dfcc', '#b08d63', '#d99a7c', '#f1e8dc', '#b9603f', '#7e3b25', '#c06a45'),
  // deep teal velvet and brushed brass
  'B-1401': p('#1f5a5c', '#184a4c', '#d9b26a', '#e8e2d6', '#e9e1d2', '#e5dccb', '#b08d63', '#2a5d60', '#ece5d8', '#d7c9ad', '#a88a5a', '#1f5a5c'),
  // mustard yellow and dark walnut
  'A-1203': p('#ece3d2', '#ddd2bd', '#d4a21f', '#e8e2d6', '#d4a21f', '#d4a21f', '#6b4a32', '#c8a77e', '#f0e8da', '#d8c8a8', '#8a6a3a', '#d4a21f'),
  // olive green and natural rattan
  'C-1205': p('#6e7448', '#5e6440', '#d8cfb8', '#a8946a', '#c9c0a3', '#c9a978', '#b89462', '#6f7a55', '#ece7dc', '#d9cdb2', '#a8946a', '#6e7448'),
  // blush pink and white oak, a blush boucle armchair
  'C-306': p('#f1ece4', '#e2dcd2', '#e5b3a6', '#f3e5dd', '#e5b3a6', '#e9b9ab', '#e9cdc3', '#e8c3b8', '#f2e9e3', '#e7cbbf', '#d29b8c', '#e5b3a6'),
  // indigo blue and natural jute
  'D-504': p('#ece5d6', '#ddd5c4', '#2f3f6b', '#e8e2d6', '#2f3f6b', '#e3dac8', '#d6cbb5', '#7a5236', '#efe9de', '#c9b28a', '#a88f63', '#2f3f6b'),
  // forest green and white marble, many indoor plants
  'D-203': { ...p('#efe9de', '#e0d9cc', '#2f5a3e', '#e8e2d6', '#2f5a3e', '#e9e2d4', '#d9d1c2', '#2f5a3e', '#ece9e0', '#d9d0bf', '#8f8a7a', '#2f5a3e'), lush: true },
  // warm greige and matte black metal
  'C-801': p('#d2c8b8', '#c3b8a6', '#8f8577', '#ebe5da', '#b9ad9a', '#e4ddd0', '#cfc5b4', '#5c4130', '#ece7df', '#cfc4b2', '#3a3836', '#3a3836'),
  // rust orange and travertine
  'D-1102': p('#b0532c', '#9a4726', '#e8dcc6', '#d08860', '#e2d4bb', '#b0532c', '#b0532c', '#6b4630', '#efe7da', '#d9c9ad', '#b0532c', '#b0532c'),
}

/** Cream and olive: the look every flat had before it got its own. */
const generic = p('#ebe3d5', '#ddd3c2', '#7e8160', '#c6b59f', '#8e8f6d', '#ebe3d5', '#b3b7a2', '#3a4d45', '#efe5d6', '#e2d6c2', '#bcab92', '#bd7f63')

const lightFor: Record<string, LightMood> = { East: 'morning', West: 'evening', South: 'midday', North: 'soft' }

export function flatTheme(u: UnitLike): FlatTheme {
  const known = palettes[u.code]
  return {
    palette: known ?? generic,
    style: u.projectName === 'Project 2' ? 'modern' : 'warm',
    light: lightFor[u.facing] ?? 'evening',
    view: u.floor <= 4 ? 'garden' : u.floor <= 9 ? 'rooftops' : 'skyline',
    mirrored: u.tower === 'B' || u.tower === 'D',
    lush: !!known?.lush,
  }
}

/** Plain words for the page, e.g. "Morning sun · garden view". */
export function describeTheme(t: FlatTheme): string {
  const light = { morning: 'Morning sun', evening: 'Evening sun', midday: 'Midday sun', soft: 'Soft daylight' }[t.light]
  const view = { garden: 'garden view', rooftops: 'rooftop view', skyline: 'skyline view' }[t.view]
  return `${light} · ${view}${t.mirrored ? ' · mirror-image plan' : ''}`
}
