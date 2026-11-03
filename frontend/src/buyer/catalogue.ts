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
  layout: Layout
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

/** Photos of furnished rooms. The cover differs per flat so the grid is not one image repeated. */
export const photos = {
  living: { src: '/images/rooms/living.jpg', name: 'Living room' },
  bedroom: { src: '/images/rooms/bedroom.jpg', name: 'Main bedroom' },
  kitchen: { src: '/images/rooms/kitchen.jpg', name: 'Kitchen' },
  balcony: { src: '/images/rooms/balcony.jpg', name: 'Balcony' },
}
const covers = [photos.living, photos.balcony, photos.bedroom, photos.kitchen]

export function coverFor(_u: Unit, index: number) {
  return covers[index % covers.length]
}

export const bathrooms = (u: Unit) => (u.bhk === 3 ? 3 : 2)

export function floorLabel(n: number) {
  if (n === 0) return 'Ground floor'
  const s = n % 10 === 1 && n % 100 !== 11 ? 'st' : n % 10 === 2 && n % 100 !== 12 ? 'nd' : n % 10 === 3 && n % 100 !== 13 ? 'rd' : 'th'
  return `${n}${s} floor`
}
