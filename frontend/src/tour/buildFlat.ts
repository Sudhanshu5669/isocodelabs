import * as THREE from 'three'
import { mergeGeometries } from 'three/addons/utils/BufferGeometryUtils.js'
import { buildFurniture, type LightAnchor } from './furniture'
import type { Mats } from './materials'
import type { Plan, Room, Wall } from './plans'

// Turns a floor plan into a 3D diorama, like an architect's cut-away model.
//
// What it builds:
//   - a thick floor slab under the whole flat, and one under each room
//   - floors with a finish per room (oak, stone tiles, balcony deck)
//   - walls with real openings: doors, windows with glass, sliding doors
//   - all furniture, sorted into the room it stands in
//
// Speed: every room's pieces are MERGED by material into one mesh each. A
// flat has hundreds of furniture parts but only about 120 meshes to draw, and
// a phone handles that easily.

export const WALL_H = 2.7
const EXT = 0.22 // exterior wall thickness
const INT = 0.12 // interior wall thickness
const SLAB = 0.32

/** A wall run plus the direction it faces out of (for cut-away hiding). */
export type FacingWall = { group: THREE.Group; normal: THREE.Vector3; center: THREE.Vector3; low?: boolean }

export type BuiltRoom = {
  def: Room
  content: THREE.Group
  slab: THREE.Mesh
  walls: FacingWall[]
  pick: THREE.Mesh
  center: THREE.Vector3
  /** Where this room's lamps are (the viewer puts a warm light at each). */
  lights: (LightAnchor & { pos: THREE.Vector3 })[]
}

export type BuiltFlat = {
  root: THREE.Group
  flatSlab: THREE.Group
  exteriorWalls: FacingWall[]
  interiorWalls: THREE.Group
  ceiling: THREE.Mesh
  rooms: Map<string, BuiltRoom>
  center: THREE.Vector3
  size: THREE.Vector3
  /** The plane that cuts interior walls down in the whole-flat view. */
  cut: THREE.Plane
  dispose: () => void
}

// ---- geometry helpers ---------------------------------------------------------

function boxAt(w: number, h: number, d: number, mat: THREE.Material, x: number, y0: number, z: number) {
  const m = new THREE.Mesh(new THREE.BoxGeometry(w, h, d), mat)
  m.position.set(x, y0 + h / 2, z)
  return m
}

/**
 * Lays a texture out at real size: each triangle takes texture coordinates
 * from the two world axes it faces (a "box projection"). A 2.5 m sofa then
 * shows 2.5 m of fabric, not one stretched copy of the weave.
 */
function projectUVs(geo: THREE.BufferGeometry, scale: number) {
  const pos = geo.attributes.position
  const nor = geo.attributes.normal
  const uv = geo.attributes.uv
  for (let i = 0; i + 2 < pos.count; i += 3) {
    const nx = Math.abs(nor.getX(i) + nor.getX(i + 1) + nor.getX(i + 2))
    const ny = Math.abs(nor.getY(i) + nor.getY(i + 1) + nor.getY(i + 2))
    const nz = Math.abs(nor.getZ(i) + nor.getZ(i + 1) + nor.getZ(i + 2))
    for (let k = i; k < i + 3; k++) {
      const x = pos.getX(k)
      const y = pos.getY(k)
      const z = pos.getZ(k)
      if (ny >= nx && ny >= nz) uv.setXY(k, x * scale, z * scale)
      else if (nx >= nz) uv.setXY(k, z * scale, y * scale)
      else uv.setXY(k, x * scale, y * scale)
    }
  }
  uv.needsUpdate = true
}

/** Merges every mesh under `src` into one mesh per material. */
function mergeByMaterial(src: THREE.Object3D, opts: { shadows: boolean }): THREE.Group {
  src.updateMatrixWorld(true)
  const inv = new THREE.Matrix4().copy(src.matrixWorld).invert()
  const byMat = new Map<THREE.Material, THREE.BufferGeometry[]>()
  src.traverse((o) => {
    const mesh = o as THREE.Mesh
    if (!mesh.isMesh) return
    let geo = mesh.geometry.clone()
    geo.applyMatrix4(new THREE.Matrix4().multiplyMatrices(inv, mesh.matrixWorld))
    // Geometries must all be the same kind (indexed or not) to merge.
    if (geo.index) geo = geo.toNonIndexed()
    for (const name of Object.keys(geo.attributes)) if (!['position', 'normal', 'uv'].includes(name)) geo.deleteAttribute(name)
    if (!geo.attributes.uv) geo.setAttribute('uv', new THREE.Float32BufferAttribute(new Float32Array((geo.attributes.position.count) * 2), 2))
    const mat = mesh.material as THREE.Material
    if (mat.userData.tile && !mat.userData.foliage) projectUVs(geo, 1 / mat.userData.tile)
    if (!byMat.has(mat)) byMat.set(mat, [])
    byMat.get(mat)!.push(geo)
    mesh.geometry.dispose()
  })
  const out = new THREE.Group()
  for (const [mat, geos] of byMat) {
    const merged = mergeGeometries(geos, false)
    geos.forEach((g) => g.dispose())
    if (!merged) continue
    const mesh = new THREE.Mesh(merged, mat)
    const glowing = mat instanceof THREE.MeshBasicMaterial
    const seeThrough = (mat as THREE.MeshStandardMaterial).transparent
    mesh.castShadow = opts.shadows && !glowing && !seeThrough
    mesh.receiveShadow = opts.shadows && !glowing
    out.add(mesh)
  }
  return out
}

/** A floor plane whose texture is laid in world space, so planks line up across rooms. */
function floorPlane(r: Room, mat: THREE.Material, scale: number) {
  const w = r.x1 - r.x0
  const d = r.z1 - r.z0
  const geo = new THREE.PlaneGeometry(w, d)
  geo.rotateX(-Math.PI / 2)
  geo.translate(r.x0 + w / 2, 0.003, r.z0 + d / 2)
  const pos = geo.attributes.position
  const uv = geo.attributes.uv
  for (let i = 0; i < pos.count; i++) uv.setXY(i, pos.getX(i) * scale, pos.getZ(i) * scale)
  return new THREE.Mesh(geo, mat)
}

// ---- walls ----------------------------------------------------------------------

/**
 * Builds one straight wall between `from` and `to` along an axis, with its
 * doors and windows. Openings outside the range are ignored.
 */
function buildWallPiece(wall: Wall, from: number, to: number, m: Mats, wallMat: THREE.Material): THREE.Group {
  const g = new THREE.Group()
  const t = wall.exterior ? EXT : INT
  const along = wall.along
  // place(len, h, y0, centreAlong) -> a box oriented along the wall
  const place = (len: number, h: number, y0: number, mid: number, mat: THREE.Material, thick = t) => {
    if (len <= 0.001 || h <= 0.001) return
    const mesh = along === 'x' ? boxAt(len, h, thick, mat, mid, y0, wall.at) : boxAt(thick, h, len, mat, wall.at, y0, mid)
    g.add(mesh)
  }

  if (wall.rail) {
    place(to - from, 0.95, 0, (from + to) / 2, m.glass, 0.02)
    place(to - from, 0.04, 0.95, (from + to) / 2, m.black, 0.05)
    place(to - from, 0.12, 0, (from + to) / 2, m.slab, 0.12)
    return g
  }

  const openings = (wall.openings ?? [])
    .map((o) => ({ ...o, from: Math.max(o.from, from), to: Math.min(o.to, to) }))
    .filter((o) => o.to - o.from > 0.05)
    .sort((a, b) => a.from - b.from)

  let cursor = from
  for (const o of openings) {
    place(o.from - cursor, WALL_H, 0, (cursor + o.from) / 2, wallMat)
    const len = o.to - o.from
    const mid = (o.from + o.to) / 2
    switch (o.kind) {
      case 'door':
        place(len, WALL_H - 2.1, 2.1, mid, wallMat)
        break
      case 'open':
        place(len, WALL_H - 2.4, 2.4, mid, wallMat)
        break
      case 'window':
      case 'high': {
        const sill = o.kind === 'high' ? 1.6 : 0.85
        const head = 2.15
        place(len, sill, 0, mid, wallMat)
        place(len, WALL_H - head, head, mid, wallMat)
        place(len, head - sill, sill, mid, m.glass, 0.02)
        place(len, 0.04, sill - 0.02, mid, m.frame, t + 0.04)
        place(0.04, head - sill, sill, o.from + 0.02, m.frame, 0.06)
        place(0.04, head - sill, sill, o.to - 0.02, m.frame, 0.06)
        place(len, 0.04, head - 0.02, mid, m.frame, 0.06)
        break
      }
      case 'slider': {
        place(len, WALL_H - 2.35, 2.35, mid, wallMat)
        place(len, 2.35, 0, mid, m.glass, 0.02)
        place(len, 0.05, 2.3, mid, m.frame, 0.08)
        place(0.05, 2.35, 0, mid, m.frame, 0.06)
        place(0.05, 2.35, 0, o.from + 0.025, m.frame, 0.06)
        place(0.05, 2.35, 0, o.to - 0.025, m.frame, 0.06)
        break
      }
    }
    cursor = o.to
  }
  place(to - cursor, WALL_H, 0, (cursor + to) / 2, wallMat)
  return g
}

// ---- bathroom tiles ---------------------------------------------------------------

/** Bathrooms get tiled walls and a tile floor. */
const isWet = (r: Room) => /bath|ensuite/.test(r.id)

/**
 * Lines a bathroom's walls with tiles, floor to ceiling, around its doors and
 * windows. Each panel is one-sided and faces into the room, so from outside
 * (the cut-away views) the near panels are invisible and only the far walls
 * show their tiles, exactly like the walls themselves.
 */
function tileLining(r: Room, plan: Plan, mat: THREE.Material): THREE.Group {
  const g = new THREE.Group()
  const edges: { along: 'x' | 'z'; at: number; from: number; to: number; inward: number }[] = [
    { along: 'x', at: r.z0, from: r.x0, to: r.x1, inward: 1 },
    { along: 'x', at: r.z1, from: r.x0, to: r.x1, inward: -1 },
    { along: 'z', at: r.x0, from: r.z0, to: r.z1, inward: 1 },
    { along: 'z', at: r.x1, from: r.z0, to: r.z1, inward: -1 },
  ]
  // Solid bands of wall (from, to heights) left beside each kind of opening.
  const bands: Record<string, [number, number][]> = {
    door: [[2.1, WALL_H]],
    open: [[2.4, WALL_H]],
    window: [[0, 0.85], [2.15, WALL_H]],
    high: [[0, 1.6], [2.15, WALL_H]],
    slider: [[2.35, WALL_H]],
  }
  for (const e of edges) {
    for (const wall of plan.walls.filter((w) => w.along === e.along && Math.abs(w.at - e.at) < 0.01 && !w.rail)) {
      const from = Math.max(wall.from, e.from)
      const to = Math.min(wall.to, e.to)
      if (to - from < 0.05) continue
      const offset = e.at + e.inward * ((wall.exterior ? EXT : INT) / 2 + 0.004)
      const panel = (a: number, b: number, y0: number, y1: number) => {
        if (b - a < 0.01 || y1 - y0 < 0.01) return
        const geo = new THREE.PlaneGeometry(b - a, y1 - y0)
        const mesh = new THREE.Mesh(geo, mat)
        const mid = (a + b) / 2
        if (e.along === 'x') {
          mesh.position.set(mid, (y0 + y1) / 2, offset)
          if (e.inward < 0) mesh.rotation.y = Math.PI // face -z
        } else {
          mesh.position.set(offset, (y0 + y1) / 2, mid)
          mesh.rotation.y = e.inward > 0 ? Math.PI / 2 : -Math.PI / 2 // face +x or -x
        }
        g.add(mesh)
      }
      const openings = (wall.openings ?? [])
        .map((o) => ({ ...o, from: Math.max(o.from, from), to: Math.min(o.to, to) }))
        .filter((o) => o.to - o.from > 0.05)
        .sort((a, b) => a.from - b.from)
      let cursor = from
      for (const o of openings) {
        panel(cursor, o.from, 0, WALL_H)
        for (const [y0, y1] of bands[o.kind] ?? []) panel(o.from, o.to, y0, y1)
        cursor = o.to
      }
      panel(cursor, to, 0, WALL_H)
    }
  }
  return g
}

// ---- the flat ---------------------------------------------------------------------

export function buildFlat(plan: Plan, m: Mats, opts: { shadows: boolean }): BuiltFlat {
  const root = new THREE.Group()
  const disposables: THREE.BufferGeometry[] = []
  const track = (g: THREE.Object3D) => {
    g.traverse((o) => {
      const mesh = o as THREE.Mesh
      if (mesh.isMesh) disposables.push(mesh.geometry)
    })
    return g
  }

  const minX = Math.min(...plan.rooms.map((r) => r.x0))
  const maxX = Math.max(...plan.rooms.map((r) => r.x1))
  const minZ = Math.min(...plan.rooms.map((r) => r.z0))
  const maxZ = Math.max(...plan.rooms.map((r) => r.z1))
  const center = new THREE.Vector3((minX + maxX) / 2, 0, (minZ + maxZ) / 2)
  const size = new THREE.Vector3(maxX - minX, WALL_H, maxZ - minZ)
  const mainCenter = new THREE.Vector3(plan.width / 2, 0, plan.depth / 2)

  // The cut-away plane: interior walls are cut at 1.15 m in the whole-flat view.
  const cut = new THREE.Plane(new THREE.Vector3(0, -1, 0), 1.15)
  const cutWall = (m.wall as THREE.MeshStandardMaterial).clone()
  cutWall.clippingPlanes = [cut]
  cutWall.clipShadows = true
  cutWall.side = THREE.DoubleSide // the cut edge shows the wall's thickness, not a hole

  // 1. The slab under the whole flat (main block plus the balcony).
  const flatSlab = new THREE.Group()
  const e = EXT / 2
  flatSlab.add(boxAt(plan.width + EXT, SLAB, plan.depth + EXT, m.slab, plan.width / 2, -SLAB, plan.depth / 2))
  for (const r of plan.rooms.filter((r) => r.z1 <= 0)) {
    flatSlab.add(boxAt(r.x1 - r.x0 + e * 2, SLAB, r.z1 - r.z0 + e, m.slab, (r.x0 + r.x1) / 2, -SLAB, (r.z0 + r.z1) / 2 - e / 2))
  }
  flatSlab.traverse((o) => ((o as THREE.Mesh).receiveShadow = true))
  root.add(track(flatSlab))

  // 2. Walls for the whole-flat view. Exterior runs stay separate so the ones
  //    facing the camera can be hidden; interior runs are merged and cut down.
  const exteriorWalls: FacingWall[] = []
  const interiorSrc = new THREE.Group()
  for (const w of plan.walls) {
    if (w.exterior || w.rail) {
      const raw = buildWallPiece(w, w.from, w.to, m, m.wall)
      const group = mergeByMaterial(raw, opts)
      const c = w.along === 'x' ? new THREE.Vector3((w.from + w.to) / 2, 1, w.at) : new THREE.Vector3(w.at, 1, (w.from + w.to) / 2)
      // Outward = away from the middle of the flat.
      const normal = w.along === 'x' ? new THREE.Vector3(0, 0, Math.sign(w.at - mainCenter.z) || -1) : new THREE.Vector3(Math.sign(w.at - mainCenter.x) || -1, 0, 0)
      exteriorWalls.push({ group, normal, center: c, low: !!w.rail })
      root.add(track(group))
    } else {
      interiorSrc.add(buildWallPiece(w, w.from, w.to, m, cutWall))
    }
  }
  const interiorWalls = mergeByMaterial(interiorSrc, opts)
  root.add(track(interiorWalls))

  // 3. Rooms: floor, furniture, their own slab and walls (for the one-room view).
  const rooms = new Map<string, BuiltRoom>()
  const contentSrc = new Map<string, THREE.Group>()
  // Bathroom tiles are cut by the same plane as the interior walls, so in the
  // whole-home view they stop where the walls stop.
  const tileCut = (m.tile as THREE.MeshStandardMaterial).clone()
  tileCut.clippingPlanes = [cut]
  for (const r of plan.rooms) {
    const g = new THREE.Group()
    const floorMat = r.floor === 'wood' ? m.oakFloor : r.floor === 'deck' ? m.deck : isWet(r) ? m.bathFloor : m.stoneFloor
    g.add(floorPlane(r, floorMat, 1 / (floorMat.userData.tile ?? 2)))
    if (isWet(r)) g.add(tileLining(r, plan, tileCut))
    contentSrc.set(r.id, g)
  }
  const roomAt = (x: number, z: number): Room => {
    const inside = plan.rooms.find((r) => x >= r.x0 && x <= r.x1 && z >= r.z0 && z <= r.z1)
    if (inside) return inside
    return plan.rooms.reduce((best, r) => {
      const d = Math.hypot((r.x0 + r.x1) / 2 - x, (r.z0 + r.z1) / 2 - z)
      const bd = Math.hypot((best.x0 + best.x1) / 2 - x, (best.z0 + best.z1) / 2 - z)
      return d < bd ? r : best
    })
  }
  for (const item of plan.furniture) {
    const piece = buildFurniture(item.t, m, item.s)
    if (!piece) continue
    piece.position.set(item.x, 0, item.z)
    piece.rotation.y = ((item.r ?? 0) * Math.PI) / 180
    contentSrc.get(roomAt(item.x, item.z).id)!.add(piece)
  }

  for (const r of plan.rooms) {
    // Lamp positions first: merging keeps only meshes.
    const src = contentSrc.get(r.id)!
    src.updateMatrixWorld(true)
    const lights: BuiltRoom['lights'] = []
    src.traverse((o) => {
      if (o.userData.light) lights.push({ ...(o.userData.light as LightAnchor), pos: o.getWorldPosition(new THREE.Vector3()) })
    })
    const content = mergeByMaterial(src, opts)
    root.add(track(content))

    const w = r.x1 - r.x0
    const d = r.z1 - r.z0
    const rc = new THREE.Vector3(r.x0 + w / 2, 0, r.z0 + d / 2)
    const slab = boxAt(w + EXT, SLAB, d + EXT, m.slab, rc.x, -SLAB, rc.z)
    slab.receiveShadow = true
    slab.visible = false
    root.add(track(slab))

    // This room's own walls: the parts of each wall run that lie on its edges.
    const walls: FacingWall[] = []
    const edges: { along: 'x' | 'z'; at: number; from: number; to: number; normal: THREE.Vector3 }[] = [
      { along: 'x', at: r.z0, from: r.x0, to: r.x1, normal: new THREE.Vector3(0, 0, -1) },
      { along: 'x', at: r.z1, from: r.x0, to: r.x1, normal: new THREE.Vector3(0, 0, 1) },
      { along: 'z', at: r.x0, from: r.z0, to: r.z1, normal: new THREE.Vector3(-1, 0, 0) },
      { along: 'z', at: r.x1, from: r.z0, to: r.z1, normal: new THREE.Vector3(1, 0, 0) },
    ]
    for (const edge of edges) {
      const runs = plan.walls.filter((wl) => wl.along === edge.along && Math.abs(wl.at - edge.at) < 0.01 && wl.to > edge.from && wl.from < edge.to)
      if (!runs.length) continue
      const src = new THREE.Group()
      for (const run of runs) src.add(buildWallPiece(run, Math.max(run.from, edge.from), Math.min(run.to, edge.to), m, m.wall))
      const group = mergeByMaterial(src, opts)
      group.visible = false
      root.add(track(group))
      const c = edge.along === 'x' ? new THREE.Vector3((edge.from + edge.to) / 2, 1, edge.at) : new THREE.Vector3(edge.at, 1, (edge.from + edge.to) / 2)
      walls.push({ group, normal: edge.normal, center: c })
    }

    const pick = new THREE.Mesh(new THREE.PlaneGeometry(w, d).rotateX(-Math.PI / 2), new THREE.MeshBasicMaterial())
    pick.position.set(rc.x, 0.02, rc.z)
    pick.visible = false
    pick.userData.roomId = r.id
    root.add(pick)
    disposables.push(pick.geometry)

    rooms.set(r.id, { def: r, content, slab, walls, pick, center: rc, lights })
  }

  // 4. A ceiling for the walk-in view. It faces down, so from above it is invisible.
  const ceiling = new THREE.Mesh(new THREE.PlaneGeometry(plan.width, plan.depth).rotateX(-Math.PI / 2), m.ceiling)
  ceiling.position.set(plan.width / 2, WALL_H, plan.depth / 2)
  ceiling.visible = false
  root.add(ceiling)
  disposables.push(ceiling.geometry)

  return {
    root,
    flatSlab,
    exteriorWalls,
    interiorWalls,
    ceiling,
    rooms,
    center,
    size,
    cut,
    dispose: () => {
      disposables.forEach((g) => g.dispose())
      rooms.forEach((r) => (r.pick.material as THREE.Material).dispose())
      cutWall.dispose()
      tileCut.dispose()
    },
  }
}
