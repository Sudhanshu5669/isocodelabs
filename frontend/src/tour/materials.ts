import * as THREE from 'three'

// Every surface in the flats uses one of these shared materials. Sharing is
// what lets buildFlat merge all pieces of the same material into one mesh.
//
// Realism comes from:
//   - photo-scanned textures (Poly Haven, CC0) for wood, fabric, plaster,
//     marble, rugs and terrazzo: colour + surface detail ("normal") maps,
//     shrunk to small WebP files in /public/textures
//   - physically based settings: fabrics have sheen, ceramic and lacquer a
//     clear coat, metals reflect the room
//   - every textured material says how many metres one texture tile covers
//     (userData.tile), and buildFlat lays textures out at that real size
//
// On slower devices ("lite") the cheaper standard material is used.

export type Mats = ReturnType<typeof createMaterials>['mats']

type Opts = { lite: boolean; anisotropy: number; onLoad: () => void }

const BASE = '/textures/'

export function createMaterials({ lite, anisotropy, onLoad }: Opts) {
  const loader = new THREE.TextureLoader()
  const cache = new Map<string, THREE.Texture>()
  const tex = (file: string, color: boolean) => {
    const key = file
    if (cache.has(key)) return cache.get(key)!
    const t = loader.load(BASE + file, onLoad)
    t.wrapS = t.wrapT = THREE.RepeatWrapping
    t.anisotropy = anisotropy
    if (color) t.colorSpace = THREE.SRGBColorSpace
    cache.set(key, t)
    return t
  }
  /** A photo-textured material. `tile` = metres covered by one copy of the texture. */
  const scanned = (
    name: string,
    tile: number,
    p: THREE.MeshPhysicalMaterialParameters & { rough?: boolean; normalStrength?: number; colorMap?: boolean } = {},
  ) => {
    // colorMap: false keeps only the surface detail (the weave, the loops) and
    // takes the colour from `color`. Fabrics look even and real that way;
    // their photos have light and dark patches that tile into a checkerboard.
    const { rough, normalStrength = 1, colorMap = true, ...rest } = p
    const params: THREE.MeshPhysicalMaterialParameters = {
      map: colorMap ? tex(`${name}_diff.webp`, true) : undefined,
      normalMap: name === 'marble_01' ? undefined : tex(`${name}_nor.webp`, false),
      normalScale: new THREE.Vector2(normalStrength, normalStrength),
      roughness: 0.8,
      ...rest,
    }
    if (rough) params.roughnessMap = tex(`${name}_rough.webp`, false)
    const m = lite || !(rest.sheen || rest.clearcoat) ? new THREE.MeshStandardMaterial(stripPhysical(params)) : new THREE.MeshPhysicalMaterial(params)
    m.userData.tile = tile
    return m
  }
  const plain = (color: string, roughness: number, metalness = 0, extra: THREE.MeshPhysicalMaterialParameters = {}) => {
    const params = { color, roughness, metalness, ...extra }
    return lite || !(extra.clearcoat || extra.sheen) ? new THREE.MeshStandardMaterial(stripPhysical(params)) : new THREE.MeshPhysicalMaterial(params)
  }

  const leafTex = leafTexture()

  const fabric = { sheen: 0.7, sheenRoughness: 0.85, sheenColor: new THREE.Color('#ffffff') }

  const mats = {
    // structure
    wall: scanned('plastered_wall_04', 2.2, { color: '#e9ddcc', roughness: 0.95, normalStrength: 0.6 }),
    slab: scanned('plastered_wall_04', 1.6, { color: '#bba892', roughness: 0.95 }),
    ceiling: plain('#f1ece4', 1, 0, { side: THREE.BackSide }),
    oakFloor: scanned('wood_floor', 2, { rough: true, roughness: 1, normalStrength: 0.7 }),
    stoneFloor: scanned('terrazzo_tiles', 1.2, { color: '#f2ede6', roughness: 0.45 }),
    deck: scanned('wood_floor_deck', 1.6, { roughness: 0.85 }),
    glass: new THREE.MeshStandardMaterial({ color: '#dfe9ea', roughness: 0.04, metalness: 0, transparent: true, opacity: 0.16, depthWrite: false }),
    frame: plain('#2a2725', 0.45, 0.6),
    // wood
    walnut: scanned('walnut_veneer', 0.9, { roughness: 0.48, clearcoat: 0.25, clearcoatRoughness: 0.4 }),
    slat: scanned('walnut_veneer', 0.6, { color: '#c9a888', roughness: 0.55 }),
    oak: scanned('oak_veneer_01', 0.9, { roughness: 0.55 }),
    trunk: scanned('oak_veneer_01', 0.3, { color: '#6d5847', roughness: 0.9 }),
    // fabric
    boucle: scanned('wool_boucle', 0.2, { colorMap: false, normalStrength: 1.4, color: '#eee6d9', roughness: 1, ...fabric }),
    boucleShade: scanned('wool_boucle', 0.2, { colorMap: false, normalStrength: 1.4, color: '#e0d6c6', roughness: 1, ...fabric }),
    linen: scanned('rough_linen', 0.45, { colorMap: false, color: '#f6f2ea', roughness: 1, ...fabric, sheen: 0.4 }),
    duvet: scanned('rough_linen', 0.45, { colorMap: false, color: '#ebe3d6', roughness: 1, ...fabric, sheen: 0.4 }),
    oliveCloth: scanned('knitted_fleece', 0.35, { colorMap: false, color: '#848564', roughness: 1, ...fabric }),
    taupe: scanned('rough_linen', 0.4, { colorMap: false, color: '#c3b29c', roughness: 1, ...fabric }),
    sage: scanned('rough_linen', 0.4, { colorMap: false, color: '#b2b5a2', roughness: 1, ...fabric }),
    rug: scanned('poly_wool_herringbone', 0.5, { colorMap: false, color: '#e3d8c6', roughness: 1, normalStrength: 0.8 }),
    rugDark: scanned('poly_wool_herringbone', 0.5, { colorMap: false, color: '#b9a991', roughness: 1 }),
    // stone, ceramic, metal
    marble: scanned('marble_01', 0.9, { rough: true, roughness: 1 }),
    stonePot: scanned('clay_plaster', 0.5, { color: '#ddd3c5', roughness: 0.95 }),
    terracotta: scanned('clay_plaster', 0.5, { color: '#c38a6c', roughness: 0.9 }),
    ceramic: plain('#f5f3ef', 0.18, 0, { clearcoat: 0.8, clearcoatRoughness: 0.1 }),
    cabinet: plain('#3a4d45', 0.42, 0, { clearcoat: 0.35, clearcoatRoughness: 0.35 }),
    black: plain('#1d1c1b', 0.38, 0.75),
    brass: plain('#b8925a', 0.28, 1),
    steel: plain('#cfd1d0', 0.3, 1),
    screen: plain('#0b0b0c', 0.12, 0.2, { clearcoat: 1, clearcoatRoughness: 0.05 }),
    mirror: plain('#e8ecec', 0.04, 1),
    // plants: real leaf shapes cut out of a small drawn texture
    leaf: new THREE.MeshStandardMaterial({ map: leafTex, color: '#8c9668', roughness: 0.7, alphaTest: 0.5, side: THREE.DoubleSide }),
    leafDark: new THREE.MeshStandardMaterial({ map: leafTex, color: '#6c7651', roughness: 0.7, alphaTest: 0.5, side: THREE.DoubleSide }),
    // art and books
    art1: scanned('rough_linen', 0.5, { colorMap: false, color: '#d8cbb6', roughness: 1, normalStrength: 0.5 }),
    art2: scanned('rough_linen', 0.5, { colorMap: false, color: '#9aa38c', roughness: 1, normalStrength: 0.5 }),
    art3: scanned('rough_linen', 0.5, { colorMap: false, color: '#bd7f63', roughness: 1, normalStrength: 0.5 }),
    art4: scanned('rough_linen', 0.5, { colorMap: false, color: '#ece4d6', roughness: 1, normalStrength: 0.5 }),
    book1: plain('#7a5944', 0.85),
    book2: plain('#cdbca3', 0.85),
    book3: plain('#55605a', 0.85),
    // light: the warm LED strips and lamp shades glow without a light source
    led: new THREE.MeshBasicMaterial({ color: '#ffcf96', toneMapped: false }),
    lampGlow: new THREE.MeshBasicMaterial({ color: '#ffe9c9', toneMapped: false }),
  }
  // Leaves keep their own texture coordinates (each leaf shows the whole drawing).
  mats.leaf.userData.foliage = true
  mats.leafDark.userData.foliage = true

  const dispose = () => {
    Object.values(mats).forEach((m) => m.dispose())
    cache.forEach((t) => t.dispose())
    leafTex.dispose()
  }
  return { mats, dispose }
}

/** Drops the physical-only settings when a standard material is used instead. */
function stripPhysical(p: THREE.MeshPhysicalMaterialParameters): THREE.MeshStandardMaterialParameters {
  const { sheen: _a, sheenRoughness: _b, sheenColor: _c, clearcoat: _d, clearcoatRoughness: _e, ...rest } = p
  return rest as THREE.MeshStandardMaterialParameters
}

/** A single olive leaf: long, narrow, with a pale midrib. Drawn once, reused by every leaf. */
function leafTexture(): THREE.CanvasTexture {
  const c = document.createElement('canvas')
  c.width = 128
  c.height = 32
  const g = c.getContext('2d')!
  const grad = g.createLinearGradient(0, 0, 0, 32)
  grad.addColorStop(0, '#d6dcc4')
  grad.addColorStop(0.5, '#ffffff')
  grad.addColorStop(1, '#c8cfb3')
  g.fillStyle = grad
  g.beginPath()
  g.moveTo(2, 16)
  g.quadraticCurveTo(60, -2, 126, 16)
  g.quadraticCurveTo(60, 34, 2, 16)
  g.fill()
  g.strokeStyle = 'rgba(255,255,240,0.7)'
  g.lineWidth = 1.2
  g.beginPath()
  g.moveTo(4, 16)
  g.lineTo(122, 16)
  g.stroke()
  const t = new THREE.CanvasTexture(c)
  t.colorSpace = THREE.SRGBColorSpace
  return t
}
