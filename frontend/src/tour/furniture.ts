import * as THREE from 'three'
import { RoundedBoxGeometry } from 'three/addons/geometries/RoundedBoxGeometry.js'
import type { Mats } from './materials'

// Furniture, modelled in code from simple shapes. No model files are
// downloaded, which keeps a flat's 3D weight at zero bytes beyond the 3D
// library itself, and every piece shares the same palette.
//
// Conventions: units are metres, each piece stands on y = 0, is centred on
// x/z, and its front faces +z (a sofa's seat, a bed's foot, a cabinet's doors).
// plans.ts rotates and places them.

type G = THREE.Group

function put(g: G, geo: THREE.BufferGeometry, mat: THREE.Material, x: number, y: number, z: number, ry = 0, rx = 0, rz = 0) {
  const m = new THREE.Mesh(geo, mat)
  m.position.set(x, y, z)
  m.rotation.set(rx, ry, rz)
  g.add(m)
  return m
}
/** A box standing on y (bottom at y). */
const box = (g: G, w: number, h: number, d: number, mat: THREE.Material, x: number, y: number, z: number, ry = 0) =>
  put(g, new THREE.BoxGeometry(w, h, d), mat, x, y + h / 2, z, ry)
/** A soft box: rounded edges are what make cushions and cabinets read as real. */
const soft = (g: G, w: number, h: number, d: number, r: number, mat: THREE.Material, x: number, y: number, z: number, ry = 0, rx = 0) =>
  put(g, new RoundedBoxGeometry(w, h, d, 2, Math.min(r, w / 2, h / 2, d / 2)), mat, x, y + h / 2, z, ry, rx)
const cyl = (g: G, rTop: number, rBot: number, h: number, mat: THREE.Material, x: number, y: number, z: number, seg = 20) =>
  put(g, new THREE.CylinderGeometry(rTop, rBot, h, seg), mat, x, y + h / 2, z)
/** A rounded blob, or (for leaf materials) a cluster of real leaves of the same size. */
const blob = (g: G, r: number, mat: THREE.Material, x: number, y: number, z: number, sx = 1, sy = 1, sz = 1) => {
  if (mat.userData.foliage) return foliage(g, mat, x, y, z, r * Math.max(sx, sz), r * sy)
  const m = put(g, new THREE.IcosahedronGeometry(r, 1), mat, x, y, z)
  m.scale.set(sx, sy, sz)
  return m
}

/**
 * Marks where a lamp's light comes from. buildFlat collects these and the
 * viewer puts a warm light at each one (the anchor itself draws nothing).
 * strength: 1 for a reading lamp, more for a pendant, less for an LED strip.
 */
export type LightAnchor = { strength: number; reach: number }
function light(g: G, x: number, y: number, z: number, strength = 1, reach = 3.2) {
  const o = new THREE.Object3D()
  o.position.set(x, y, z)
  o.userData.light = { strength, reach } satisfies LightAnchor
  g.add(o)
}

// One shared leaf shape: about 9 cm long, like an olive leaf. Each leaf is a
// small card showing the leaf drawing (transparent around it).
const leafGeo = new THREE.PlaneGeometry(0.13, 0.036)

/** Scatters leaves through an egg shape of radius rw (across) and rh (up). */
function foliage(g: G, mat: THREE.Material, x: number, y: number, z: number, rw: number, rh: number) {
  const rand = seeded(Math.round(Math.abs(x * 997 + y * 389 + z * 151)) + 3)
  const count = Math.min(220, Math.round(55 + rw * rh * 1700))
  let last: THREE.Mesh | null = null
  for (let i = 0; i < count; i++) {
    // a random point inside the egg, denser towards the outside like real foliage
    const a = rand() * Math.PI * 2
    const b = Math.acos(2 * rand() - 1)
    const d = Math.cbrt(0.35 + rand() * 0.65)
    const leaf = new THREE.Mesh(leafGeo, mat)
    leaf.position.set(x + Math.sin(b) * Math.cos(a) * rw * d, y + Math.cos(b) * rh * d, z + Math.sin(b) * Math.sin(a) * rw * d)
    leaf.rotation.set(rand() * Math.PI, rand() * Math.PI * 2, rand() * Math.PI)
    g.add(leaf)
    last = leaf
  }
  return last!
}

/** A thin branch between two points. */
function branch(g: G, from: THREE.Vector3, to: THREE.Vector3, r: number, mat: THREE.Material) {
  const len = from.distanceTo(to)
  const m = new THREE.Mesh(new THREE.CylinderGeometry(r * 0.6, r, len, 5), mat)
  m.position.copy(from).add(to).multiplyScalar(0.5)
  m.quaternion.setFromUnitVectors(new THREE.Vector3(0, 1, 0), to.clone().sub(from).normalize())
  g.add(m)
}

/** Same numbers every time for the same seed, so plants look the same on every visit. */
function seeded(seed: number) {
  let s = seed * 9973 + 17
  return () => ((s = (s * 16807) % 2147483647) / 2147483647)
}

type Builder = (m: Mats, s: number[]) => G

const builders: Record<string, Builder> = {
  rug(m, [w = 2.4, d = 1.8]) {
    const g = new THREE.Group()
    box(g, w, 0.01, d, m.rugDark, 0, 0, 0)
    box(g, w - 0.14, 0.013, d - 0.14, m.rug, 0, 0, 0)
    return g
  },

  // Vertical walnut slats with warm LED lines, as in the reference's TV wall.
  slatWall(m, [w = 3.2]) {
    const g = new THREE.Group()
    const h = 2.55
    box(g, w, h, 0.02, m.walnut, 0, 0, 0)
    const pitch = 0.085
    for (let x = -w / 2 + 0.03; x < w / 2 - 0.02; x += pitch) box(g, 0.045, h, 0.035, m.slat, x, 0, 0.025)
    box(g, 0.012, h - 0.1, 0.012, m.led, -w / 2 - 0.01, 0.05, 0.03)
    box(g, 0.012, h - 0.1, 0.012, m.led, w / 2 + 0.01, 0.05, 0.03)
    box(g, w, 0.012, 0.012, m.led, 0, h - 0.04, 0.05)
    return g
  },

  // A floating walnut console with a glow beneath it, and a wall-hung TV.
  tvUnit(m, [len = 2.0]) {
    const g = new THREE.Group()
    soft(g, len, 0.3, 0.42, 0.012, m.walnut, 0, 0.3, 0)
    box(g, len - 0.12, 0.008, 0.012, m.led, 0, 0.29, 0.12)
    box(g, 1.5, 0.86, 0.03, m.black, 0, 1.0, -0.16)
    box(g, 1.46, 0.82, 0.01, m.tv, 0, 1.02, -0.14)
    light(g, 0, 0.22, 0.25, 0.35, 2.2) // the LED glow under the console
    box(g, 0.26, 0.05, 0.18, m.book2, len / 2 - 0.35, 0.6, 0)
    box(g, 0.22, 0.04, 0.16, m.book3, len / 2 - 0.35, 0.65, 0)
    cyl(g, 0.05, 0.07, 0.2, m.stonePot, -len / 2 + 0.3, 0.6, 0)
    blob(g, 0.09, m.leaf, -len / 2 + 0.3, 0.88, 0, 1, 1.3, 1)
    return g
  },

  // A deep cream sectional with a chaise on its right (+x) end.
  sectional(m, [len = 2.5]) {
    const g = new THREE.Group()
    const D = 0.95
    soft(g, len, 0.36, D, 0.05, m.boucle, 0, 0.04, 0)
    soft(g, len, 0.42, 0.22, 0.09, m.boucle, 0, 0.36, -D / 2 + 0.11)
    soft(g, 0.24, 0.26, D, 0.09, m.boucle, -len / 2 + 0.12, 0.36, 0)
    const n = 3
    const cw = (len - 0.24) / n
    for (let i = 0; i < n; i++) {
      const x = -len / 2 + 0.24 + cw * i + cw / 2
      soft(g, cw - 0.02, 0.13, D - 0.26, 0.06, m.boucle, x, 0.38, 0.12)
      soft(g, cw - 0.06, 0.38, 0.17, 0.08, m.boucleShade, x, 0.48, -D / 2 + 0.28, 0, -0.14)
    }
    // the chaise
    soft(g, 0.95, 0.36, 0.95, 0.05, m.boucle, len / 2 - 0.475, 0.04, D / 2 + 0.47)
    soft(g, 0.91, 0.13, 0.9, 0.06, m.boucle, len / 2 - 0.475, 0.38, D / 2 + 0.47)
    // cushions and a throw
    soft(g, 0.42, 0.4, 0.13, 0.07, m.oliveCloth, -len / 2 + 0.5, 0.5, -D / 2 + 0.42, 0.25, -0.2)
    soft(g, 0.4, 0.38, 0.13, 0.07, m.taupe, -len / 2 + 0.92, 0.5, -D / 2 + 0.4, -0.1, -0.2)
    soft(g, 0.4, 0.38, 0.13, 0.07, m.oliveCloth, len / 2 - 0.5, 0.5, -D / 2 + 0.4, -0.2, -0.2)
    box(g, 0.55, 0.025, 0.95, m.knit, len / 2 - 0.3, 0.505, D / 2 + 0.45)
    box(g, 0.55, 0.32, 0.02, m.knit, len / 2 - 0.3, 0.2, D / 2 + 0.93)
    return g
  },

  armchair(m) {
    const g = new THREE.Group()
    soft(g, 0.85, 0.36, 0.82, 0.06, m.boucle, 0, 0.06, 0)
    soft(g, 0.85, 0.4, 0.18, 0.08, m.boucle, 0, 0.36, -0.32)
    soft(g, 0.16, 0.24, 0.82, 0.07, m.boucle, -0.345, 0.36, 0)
    soft(g, 0.16, 0.24, 0.82, 0.07, m.boucle, 0.345, 0.36, 0)
    soft(g, 0.52, 0.11, 0.58, 0.05, m.boucleShade, 0, 0.42, 0.06)
    soft(g, 0.36, 0.32, 0.12, 0.06, m.taupe, 0, 0.5, -0.18, 0, -0.2)
    return g
  },

  coffeeTable(m) {
    const g = new THREE.Group()
    cyl(g, 0.5, 0.5, 0.06, m.walnut, 0, 0.34, 0, 40)
    cyl(g, 0.22, 0.27, 0.34, m.walnut, 0, 0, 0, 28)
    box(g, 0.24, 0.05, 0.17, m.book2, 0.14, 0.4, 0.06)
    box(g, 0.2, 0.04, 0.15, m.book1, 0.14, 0.45, 0.06, 0.2)
    cyl(g, 0.05, 0.07, 0.17, m.stonePot, -0.14, 0.4, -0.05)
    blob(g, 0.08, m.leaf, -0.14, 0.64, -0.05, 1, 1.4, 1)
    cyl(g, 0.09, 0.06, 0.05, m.black, -0.05, 0.4, 0.2)
    return g
  },

  // A black arc lamp: pole, arm reaching forward (+z), dome shade.
  arcLamp(m) {
    const g = new THREE.Group()
    cyl(g, 0.15, 0.16, 0.04, m.black, 0, 0, 0, 24)
    cyl(g, 0.013, 0.013, 1.72, m.black, 0, 0.04, 0, 8)
    box(g, 0.022, 0.022, 0.5, m.black, 0, 1.73, 0.24)
    put(g, new THREE.SphereGeometry(0.17, 20, 8, 0, Math.PI * 2, 0, Math.PI / 2), m.black, 0, 1.56, 0.5)
    put(g, new THREE.CircleGeometry(0.15, 20), m.lampGlow, 0, 1.565, 0.5, 0, Math.PI / 2)
    light(g, 0, 1.4, 0.5, 1.2)
    return g
  },

  // An olive tree in a stone pot (a bushy plant when short).
  plant(m, [h = 1.4]) {
    const g = new THREE.Group()
    const rand = seeded(Math.round(h * 100))
    const potR = 0.12 + h * 0.06
    const potH = 0.22 + h * 0.1
    cyl(g, potR, potR * 0.82, potH, m.stonePot, 0, 0, 0, 22)
    cyl(g, potR * 0.9, potR * 0.9, 0.01, m.trunk, 0, potH - 0.02, 0, 22)
    if (h >= 1) {
      // An olive tree: a slim trunk, branches, and clusters of small leaves.
      cyl(g, 0.022, 0.035, h * 0.62, m.trunk, 0, potH, 0, 7)
      const top = potH + h * 0.55
      const fork = new THREE.Vector3(0, potH + h * 0.5, 0)
      for (let i = 0; i < 9; i++) {
        const a = rand() * Math.PI * 2
        const rr = 0.08 + rand() * 0.24 * h
        const y = top + rand() * h * 0.42
        const end = new THREE.Vector3(Math.cos(a) * rr, y, Math.sin(a) * rr)
        branch(g, fork, end, 0.012, m.trunk)
        blob(g, 0.12 + rand() * 0.08, i % 3 ? m.leaf : m.leafDark, end.x, end.y, end.z, 1.4, 0.8, 1.4)
      }
    } else {
      for (let i = 0; i < 7; i++) {
        const a = (i / 7) * Math.PI * 2
        blob(g, 0.11 + rand() * 0.05, i % 2 ? m.leaf : m.leafDark, Math.cos(a) * 0.1, potH + 0.1 + rand() * h * 0.5, Math.sin(a) * 0.1, 1, 1.3, 1)
      }
    }
    return g
  },

  // Open walnut shelves with books, vases and a warm light under each shelf.
  shelf(m, [w = 0.9]) {
    const g = new THREE.Group()
    const rand = seeded(5)
    const d = 0.32
    box(g, 0.03, 2.1, d, m.walnut, -w / 2, 0, 0)
    box(g, 0.03, 2.1, d, m.walnut, w / 2, 0, 0)
    const levels = [0.05, 0.47, 0.89, 1.31, 1.73, 2.08]
    levels.forEach((y) => box(g, w, 0.025, d, m.walnut, 0, y, 0))
    levels.slice(1).forEach((y) => box(g, w - 0.06, 0.008, 0.01, m.led, 0, y - 0.01, d / 2 - 0.03))
    const books = [m.book1, m.book2, m.book3, m.art1]
    levels.slice(0, 5).forEach((y, i) => {
      if (i % 2 === 0) {
        let x = -w / 2 + 0.06
        for (let k = 0; k < 5; k++) {
          const bw = 0.03 + rand() * 0.025
          const bh = 0.2 + rand() * 0.1
          box(g, bw, bh, 0.2, books[k % books.length], x + bw / 2, y + 0.025, 0)
          x += bw + 0.004
        }
        cyl(g, 0.06, 0.07, 0.18, m.stonePot, w / 2 - 0.15, y + 0.025, 0)
      } else {
        cyl(g, 0.05, 0.08, 0.24, m.black, -w / 4, y + 0.025, 0)
        box(g, 0.22, 0.06, 0.18, m.book2, w / 4, y + 0.025, 0)
        blob(g, 0.07, m.leaf, -w / 4, y + 0.33, 0, 1, 1.4, 1)
      }
    })
    return g
  },

  // Wall art: s = [width, height, style]; style 4 is a tall mirror.
  art(m, [w = 1, h = 0.7, style = 1]) {
    const g = new THREE.Group()
    const y = style === 4 ? 0.55 : 1.55 - h / 2
    if (style === 4) {
      box(g, w, h, 0.025, m.brass, 0, y, 0)
      box(g, w - 0.04, h - 0.04, 0.01, m.mirror, 0, y + 0.02, 0.012)
      return g
    }
    box(g, w, h, 0.03, style === 3 ? m.oak : m.black, 0, y, 0)
    const canvas = style === 2 ? m.art2 : style === 3 ? m.art4 : m.art1
    box(g, w - 0.05, h - 0.05, 0.012, canvas, 0, y + 0.025, 0.015)
    // a simple abstract block, like the reference's print
    const accent = style === 2 ? m.art4 : style === 3 ? m.art3 : m.art4
    box(g, (w - 0.05) * 0.45, (h - 0.05) * 0.5, 0.004, accent, -w * 0.12, y + 0.025 + h * 0.08, 0.023)
    box(g, (w - 0.05) * 0.3, (h - 0.05) * 0.28, 0.004, m.art2, w * 0.18, y + 0.025 + h * 0.5, 0.023)
    return g
  },

  diningTable(m, [len = 1.6, w = 0.9]) {
    const g = new THREE.Group()
    soft(g, len, 0.045, w, 0.012, m.oak, 0, 0.73, 0)
    box(g, 0.08, 0.73, w - 0.24, m.walnut, -len / 2 + 0.28, 0, 0)
    box(g, 0.08, 0.73, w - 0.24, m.walnut, len / 2 - 0.28, 0, 0)
    box(g, len - 0.56, 0.06, 0.06, m.walnut, 0, 0.18, 0)
    cyl(g, 0.06, 0.08, 0.22, m.stoneware, 0, 0.775, 0)
    blob(g, 0.1, m.leaf, 0, 1.05, 0, 1.2, 1.5, 1.2)
    return g
  },

  chair(m) {
    const g = new THREE.Group()
    for (const [x, z] of [[-0.19, -0.19], [0.19, -0.19], [-0.19, 0.19], [0.19, 0.19]]) cyl(g, 0.014, 0.014, 0.44, m.black, x, 0, z, 6)
    soft(g, 0.46, 0.04, 0.46, 0.015, m.oak, 0, 0.44, 0)
    soft(g, 0.42, 0.04, 0.42, 0.018, m.boucle, 0, 0.48, 0.01)
    soft(g, 0.44, 0.28, 0.035, 0.012, m.oak, 0, 0.62, -0.21, 0, -0.08)
    return g
  },

  pendant(m) {
    const g = new THREE.Group()
    cyl(g, 0.004, 0.004, 0.9, m.black, 0, 1.8, 0, 4)
    put(g, new THREE.CylinderGeometry(0.05, 0.17, 0.18, 24, 1, true), m.brass, 0, 1.75, 0)
    put(g, new THREE.CircleGeometry(0.16, 24), m.lampGlow, 0, 1.665, 0, 0, Math.PI / 2)
    light(g, 0, 1.55, 0, 1.1)
    return g
  },

  sideboard(m, [len = 1.4]) {
    const g = new THREE.Group()
    for (const x of [-len / 2 + 0.08, len / 2 - 0.08]) for (const z of [-0.14, 0.14]) cyl(g, 0.012, 0.012, 0.14, m.brass, x, 0, z, 6)
    soft(g, len, 0.6, 0.42, 0.012, m.walnut, 0, 0.14, 0)
    for (let i = 1; i < 3; i++) box(g, 0.004, 0.54, 0.004, m.black, -len / 2 + (len / 3) * i, 0.17, 0.212)
    cyl(g, 0.07, 0.09, 0.3, m.stonePot, -len / 2 + 0.25, 0.74, 0)
    blob(g, 0.1, m.leaf, -len / 2 + 0.25, 1.15, 0, 1.2, 1.6, 1.2)
    box(g, 0.3, 0.06, 0.2, m.book2, len / 2 - 0.3, 0.74, 0)
    cyl(g, 0.06, 0.08, 0.06, m.black, len / 2 - 0.3, 0.8, 0)
    return g
  },

  loungeChair(m) {
    const g = new THREE.Group()
    box(g, 0.72, 0.22, 0.72, m.oak, 0, 0.03, 0)
    soft(g, 0.68, 0.11, 0.68, 0.04, m.taupe, 0, 0.25, 0.01)
    soft(g, 0.68, 0.5, 0.12, 0.05, m.boucle, 0, 0.3, -0.3, 0, -0.25)
    return g
  },

  sideTable(m) {
    const g = new THREE.Group()
    cyl(g, 0.24, 0.24, 0.04, m.terracotta, 0, 0.42, 0, 28)
    cyl(g, 0.05, 0.08, 0.42, m.black, 0, 0, 0, 12)
    cyl(g, 0.04, 0.05, 0.12, m.stoneware, 0.06, 0.46, 0.05)
    return g
  },

  planter(m, [len = 1.4]) {
    const g = new THREE.Group()
    const rand = seeded(11)
    box(g, len, 0.42, 0.34, m.stonePot, 0, 0, 0)
    for (let x = -len / 2 + 0.15; x < len / 2 - 0.1; x += 0.2) blob(g, 0.13 + rand() * 0.06, rand() > 0.5 ? m.leaf : m.leafDark, x, 0.52 + rand() * 0.1, 0, 1.1, 1.3, 1)
    return g
  },

  fridge(m) {
    const g = new THREE.Group()
    soft(g, 0.72, 1.92, 0.66, 0.02, m.steel, 0, 0, 0)
    box(g, 0.72, 0.004, 0.01, m.black, 0, 1.25, 0.331)
    box(g, 0.02, 0.5, 0.03, m.black, 0.3, 1.35, 0.34)
    box(g, 0.02, 0.4, 0.03, m.black, 0.3, 0.7, 0.34)
    return g
  },

  // A run of kitchen base cabinets: s = [length, sink, hob, upper cabinets].
  kitchenRun(m, [len = 3, sink = 0, hob = 0, uppers = 0]) {
    const g = new THREE.Group()
    box(g, len - 0.04, 0.1, 0.54, m.black, 0, 0, -0.03)
    box(g, len, 0.76, 0.6, m.cabinet, 0, 0.1, 0)
    soft(g, len + 0.02, 0.04, 0.63, 0.006, m.marble, 0, 0.86, 0.015)
    const doors = Math.max(2, Math.round(len / 0.6))
    for (let i = 1; i < doors; i++) box(g, 0.005, 0.7, 0.004, m.black, -len / 2 + (len / doors) * i, 0.13, 0.301)
    for (let i = 0; i < doors; i++) box(g, 0.22, 0.012, 0.014, m.brass, -len / 2 + (len / doors) * (i + 0.5), 0.8, 0.31)
    box(g, len, 0.55, 0.012, m.tile, 0, 0.9, -0.294) // tiled splashback
    if (sink) {
      box(g, 0.62, 0.012, 0.42, m.steel, -len * 0.12, 0.895, 0.02)
      cyl(g, 0.014, 0.018, 0.3, m.brass, -len * 0.12, 0.9, -0.2, 8)
      box(g, 0.02, 0.02, 0.2, m.brass, -len * 0.12, 1.18, -0.11)
    }
    if (hob) box(g, 0.6, 0.01, 0.5, m.screen, len * 0.1, 0.9, 0.02)
    if (uppers) {
      box(g, len, 0.72, 0.34, m.cabinet, 0, 1.48, -0.13)
      box(g, len - 0.06, 0.008, 0.01, m.led, 0, 1.47, 0.02)
      for (let i = 1; i < doors; i++) box(g, 0.005, 0.66, 0.004, m.black, -len / 2 + (len / doors) * i, 1.51, 0.041)
    }
    cyl(g, 0.05, 0.06, 0.18, m.stoneware, len / 2 - 0.25, 0.9, -0.15)
    blob(g, 0.08, m.leaf, len / 2 - 0.25, 1.15, -0.15, 1, 1.4, 1)
    return g
  },

  stool(m) {
    const g = new THREE.Group()
    cyl(g, 0.17, 0.17, 0.05, m.walnut, 0, 0.6, 0, 24)
    cyl(g, 0.022, 0.022, 0.6, m.black, 0, 0, 0, 8)
    cyl(g, 0.18, 0.18, 0.015, m.black, 0, 0, 0, 24)
    return g
  },

  // A bed: s = [width, length]. The headboard is at -z, the foot at +z.
  bed(m, [w = 1.8, l = 2.1]) {
    const g = new THREE.Group()
    soft(g, w + 0.08, 0.28, l, 0.025, m.walnut, 0, 0.05, 0)
    soft(g, w, 0.2, l - 0.06, 0.06, m.linen, 0, 0.3, 0.02)
    soft(g, w + 0.05, 0.07, l * 0.64, 0.035, m.duvet, 0, 0.465, l / 2 - l * 0.32 + 0.02)
    box(g, w + 0.07, 0.025, 0.46, m.knit, 0, 0.53, l / 2 - 0.32)
    for (const x of [-w / 4, w / 4]) soft(g, w / 2 - 0.12, 0.14, 0.42, 0.07, m.linen, x, 0.48, -l / 2 + 0.3, 0, -0.25)
    soft(g, 0.42, 0.3, 0.12, 0.06, m.taupe, -0.18, 0.5, -l / 2 + 0.56, 0.1, -0.3)
    soft(g, 0.36, 0.28, 0.12, 0.06, m.oliveCloth, 0.2, 0.5, -l / 2 + 0.58, -0.15, -0.3)
    soft(g, w + 0.34, 1.08, 0.1, 0.045, m.sage, 0, 0.08, -l / 2 - 0.06)
    return g
  },

  nightstand(m) {
    const g = new THREE.Group()
    soft(g, 0.48, 0.44, 0.4, 0.015, m.walnut, 0, 0.06, 0)
    box(g, 0.44, 0.004, 0.004, m.black, 0, 0.32, 0.201)
    cyl(g, 0.045, 0.065, 0.22, m.stoneware, -0.08, 0.5, -0.04, 16)
    cyl(g, 0.1, 0.13, 0.17, m.shade, -0.08, 0.72, -0.04, 20)
    put(g, new THREE.CircleGeometry(0.12, 16), m.lampGlow, -0.08, 0.715, -0.04, 0, Math.PI / 2)
    light(g, -0.08, 0.8, -0.04, 0.8, 2.8)
    box(g, 0.16, 0.04, 0.12, m.book1, 0.12, 0.5, 0.04)
    return g
  },

  wardrobe(m, [w = 1.8]) {
    const g = new THREE.Group()
    soft(g, w, 2.25, 0.6, 0.012, m.oak, 0, 0.04, 0)
    const doors = Math.max(2, Math.round(w / 0.5))
    for (let i = 1; i < doors; i++) box(g, 0.004, 2.15, 0.004, m.black, -w / 2 + (w / doors) * i, 0.09, 0.301)
    for (let i = 0; i < doors; i += 2) {
      const x = -w / 2 + (w / doors) * (i + 1)
      box(g, 0.012, 0.5, 0.02, m.black, x - 0.03, 0.95, 0.31)
      box(g, 0.012, 0.5, 0.02, m.black, x + 0.03, 0.95, 0.31)
    }
    return g
  },

  desk(m) {
    const g = new THREE.Group()
    soft(g, 1.2, 0.035, 0.58, 0.008, m.oak, 0, 0.74, 0)
    box(g, 0.03, 0.74, 0.54, m.black, -0.57, 0, 0)
    box(g, 0.03, 0.74, 0.54, m.black, 0.57, 0, 0)
    box(g, 0.34, 0.012, 0.24, m.black, -0.1, 0.775, 0.02)
    box(g, 0.34, 0.22, 0.01, m.screen, -0.1, 0.787, -0.1)
    cyl(g, 0.05, 0.06, 0.32, m.black, 0.42, 0.775, -0.15, 12)
    put(g, new THREE.ConeGeometry(0.09, 0.12, 16, 1, true), m.brass, 0.42, 1.12, -0.12, 0, 0.4)
    light(g, 0.42, 1.0, -0.06, 0.7, 2.4)
    return g
  },

  vanity(m) {
    const g = new THREE.Group()
    soft(g, 0.9, 0.45, 0.48, 0.015, m.walnut, 0, 0.38, 0)
    soft(g, 0.92, 0.03, 0.5, 0.008, m.marble, 0, 0.83, 0)
    soft(g, 0.44, 0.12, 0.36, 0.06, m.ceramic, 0, 0.86, 0.02)
    cyl(g, 0.012, 0.016, 0.2, m.brass, 0, 0.86, -0.2, 8)
    put(g, new THREE.CylinderGeometry(0.32, 0.32, 0.02, 36), m.mirror, 0, 1.55, -0.23, 0, Math.PI / 2)
    put(g, new THREE.TorusGeometry(0.325, 0.012, 6, 40), m.brass, 0, 1.55, -0.225)
    return g
  },

  shower(m, [size = 0.9]) {
    const g = new THREE.Group()
    box(g, size, 0.05, size, m.ceramic, 0, 0, 0)
    box(g, size * 0.62, 2.0, 0.012, m.glass, -size * 0.19, 0.05, size / 2 - 0.01)
    box(g, size * 0.62, 0.02, 0.02, m.black, -size * 0.19, 2.04, size / 2 - 0.01)
    box(g, 0.02, 0.02, 0.28, m.brass, 0, 2.05, -size / 2 + 0.14)
    cyl(g, 0.11, 0.11, 0.012, m.brass, 0, 2.03, -size / 2 + 0.3, 24)
    return g
  },

  toilet(m) {
    const g = new THREE.Group()
    soft(g, 0.37, 0.34, 0.54, 0.12, m.ceramic, 0, 0.06, 0.02)
    soft(g, 0.38, 0.035, 0.5, 0.1, m.ceramic, 0, 0.41, 0.03)
    box(g, 0.22, 0.15, 0.012, m.brass, 0, 0.95, -0.27)
    return g
  },
}

/** Builds one piece of furniture, or nothing (with a warning) for an unknown name. */
export function buildFurniture(type: string, m: Mats, s: number[] = []): G | null {
  const b = builders[type]
  if (!b) {
    console.warn('tour: unknown furniture', type)
    return null
  }
  return b(m, s)
}
