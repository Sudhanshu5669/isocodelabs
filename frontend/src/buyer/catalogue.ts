import type { Layout } from '../tour/plans'

// Catalogue data and helpers shared by the listing and the flat page.

// store.Unit
export type Unit = {
  code: string
  projectId: string
  projectName: string
  locality: string
  reraNumber: string
  bhk: 2 | 3
  layout: Layout | null // null: no 3D model yet, the flat page shows photos
  tower: string
  floor: number
  areaSqft: number
  facing: string
  indicativePriceLakh: number
}

// GET /api/units/{code}
export type UnitDetail = Unit & {
  address: string
  latitude: number
  longitude: number
  facts: { topic: string; label: string; value: string }[]
}

/** "₹72 L" or "₹1.08 Cr" */
export function lakhs(l: number): string {
  return l >= 100 ? `₹${(l / 100).toFixed(2).replace(/0$/, '')} Cr` : `₹${l} L`
}

/** Generic photos of a furnished home (chat page, and any flat without its own photos). */
export const photos = {
  living: { src: '/images/rooms/living.jpg', name: 'Living room' },
  bedroom: { src: '/images/rooms/bedroom.jpg', name: 'Main bedroom' },
  kitchen: { src: '/images/rooms/kitchen.jpg', name: 'Kitchen' },
  balcony: { src: '/images/rooms/balcony.jpg', name: 'Balcony' },
}

export type Photo = { src: string; name: string; fallback: string }

/**
 * Each flat's own photos, in /images/flats/<code>/. They follow the flat's real
 * details: the light matches the way it faces (morning sun for east, evening
 * for west, soft daylight for north, midday for south), the view matches its
 * floor (garden, rooftops or skyline), and each flat has its own palette.
 * `fallback` is the generic photo of the same room, used if a flat has none.
 */
export function photosFor(u: Pick<Unit, 'code'>): Photo[] {
  return (Object.keys(photos) as (keyof typeof photos)[]).map((room) => ({
    src: `/images/flats/${encodeURIComponent(u.code)}/${room}.jpg`,
    name: photos[room].name,
    fallback: photos[room].src,
  }))
}

/** The card cover: the flat's own living room. */
export function coverFor(u: Pick<Unit, 'code'>): Photo {
  return photosFor(u)[0]
}

/** For <img onError>: swap a missing per-flat photo for the generic one, once. */
export function showFallback(e: { currentTarget: HTMLImageElement }, fallback: string) {
  const img = e.currentTarget
  if (!img.src.endsWith(fallback)) img.src = fallback
}

export const bathrooms = (u: Unit) => (u.bhk === 3 ? 3 : 2)

export function floorLabel(n: number) {
  if (n === 0) return 'Ground floor'
  const s = n % 10 === 1 && n % 100 !== 11 ? 'st' : n % 10 === 2 && n % 100 !== 12 ? 'nd' : n % 10 === 3 && n % 100 !== 13 ? 'rd' : 'th'
  return `${n}${s} floor`
}
