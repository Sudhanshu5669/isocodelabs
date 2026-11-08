import * as THREE from 'three'
import type { FlatTheme } from './themes'

// Every surface in the flats uses one of these shared materials. Sharing is
// what lets buildFlat merge all pieces of the same material into one mesh.
//
// Realism comes from:
//   - photo-scanned textures (Poly Haven, CC0) for wood, fabric, plaster,
//     tiles, rugs and terrazzo: colour, surface detail ("normal") and
//     roughness maps as WebP files in /public/textures, 1024 px on desktops
//     and 512 px copies (*_s.webp) on phones and slower devices
//   - made-in-code textures where no scan fits: white marble for worktops,
//     60 cm vitrified floor tiles, brushed brass and steel, powder-coated
//     black metal, speckled stoneware, and the picture on the TV
//   - warm light: lamp shades and LED strips glow amber (the lights
//     themselves are added by TourViewer at the anchors furniture.ts sets)
//   - physically based settings: fabrics have sheen, ceramic and lacquer a
//     clear coat, metals reflect the room
//   - every textured material says how many metres one texture tile covers
//     (userData.tile), and buildFlat lays textures out at that real size
//
// On slower devices ("lite") the cheaper standard material is used.

export type Mats = ReturnType<typeof createMaterials>['mats']

// Each flat is dressed to match its photos (themes.ts): its palette colours
// the upholstery, cushions, throws, kitchen, rugs, walls and art, and its
// project sets the floors and wood. Project 1 (warm): pale terrazzo floors and
// light oak; Project 2 (modern): large matte porcelain tiles and walnut.
type Opts = { lite: boolean; anisotropy: number; onLoad: () => void; theme: FlatTheme }

const BASE = '/textures/'

export function createMaterials({ lite, anisotropy, onLoad, theme }: Opts) {
  const c = theme.palette
  const warm = theme.style === 'warm'
  const loader = new THREE.TextureLoader()
  const cache = new Map<string, THREE.Texture>()
  const tex = (name: string, map: 'diff' | 'nor' | 'rough' | 'ao', color: boolean) => {
    const key = `${name}_${map}${lite ? '_s' : ''}.webp`
    if (cache.has(key)) return cache.get(key)!
    const t = loader.load(BASE + key, onLoad)
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
    p: THREE.MeshPhysicalMaterialParameters & { rough?: boolean; normalStrength?: number; colorMap?: boolean; normal?: boolean; weave?: boolean } = {},
  ) => {
    // colorMap: false keeps only the surface detail (the weave, the loops) and
    // takes the colour from `color`. Fabrics look even and real that way;
    // their photos have light and dark patches that tile into a checkerboard.
    // rough: true also uses the scan's roughness map (where light catches).
    // weave: true shades the colour with the fabric's own pattern (*_ao.webp,
    // made from its normal map with large patches filtered out), so a sofa or
    // a rug still reads as woven from across the room.
    const { rough, normalStrength = 1, colorMap = true, normal = true, weave = false, ...rest } = p
    const params: THREE.MeshPhysicalMaterialParameters = {
      map: colorMap ? tex(name, 'diff', true) : weave ? tex(name, 'ao', false) : undefined,
      normalMap: normal ? tex(name, 'nor', false) : undefined,
      normalScale: new THREE.Vector2(normalStrength, normalStrength),
      roughness: 0.8,
      ...rest,
    }
    if (rough) params.roughnessMap = tex(name, 'rough', false)
    if (params.sheen && !params.sheenColor) params.sheenColor = new THREE.Color(params.color ?? '#ffffff')
    const m = lite || !(rest.sheen || rest.clearcoat) ? new THREE.MeshStandardMaterial(stripPhysical(params)) : new THREE.MeshPhysicalMaterial(params)
    m.userData.tile = tile
    return m
  }
  const plain = (color: string, roughness: number, metalness = 0, extra: THREE.MeshPhysicalMaterialParameters = {}) => {
    const params = { color, roughness, metalness, ...extra }
    return lite || !(extra.clearcoat || extra.sheen) ? new THREE.MeshStandardMaterial(stripPhysical(params)) : new THREE.MeshPhysicalMaterial(params)
  }

  const leafTex = leafTexture()

  // Sheen: the soft glow cloth has at grazing angles. Kept light and tinted
  // with the cloth's own colour (see scanned), or beds and rugs seen from
  // above wash out to a flat pale grey.
  const fabric = { sheen: 0.3, sheenRoughness: 0.8 }

  // Brushed metal: fine streaks in the normal and roughness maps; colour comes from `color`.
  const metal = (color: string, roughness: number, metalness = 1) =>
    scanned('brushed', 0.25, { colorMap: false, rough: true, color, roughness, metalness, normalStrength: 0.25 })

  // Tile sizes are the scans' real sizes in metres (from Poly Haven), so a
  // floor tile or a weave is drawn at its true size.
  // Powder-coated metal: a fine orange-peel surface with a satin sheen.
  const powder = (color: string) =>
    scanned('powder', 0.12, { colorMap: false, rough: true, color, roughness: 0.75, metalness: 0.35, normalStrength: 0.5 })

  // The TV shows a warm painting, like a frame TV, and glows a little.
  const tvArt = new THREE.TextureLoader().load(BASE + 'tv_art.webp', onLoad)
  tvArt.colorSpace = THREE.SRGBColorSpace

  const mats = {
    // structure
    wall: scanned('white_stucco', 2, { rough: true, color: c.wall, roughness: 1, normalStrength: 0.45 }),
    slab: scanned('clay_plaster', 2, { rough: true, color: '#b9a690', roughness: 1 }),
    ceiling: scanned('white_stucco', 2, { colorMap: false, color: '#f4efe7', roughness: 1, normalStrength: 0.25, side: THREE.BackSide }),
    // living rooms and bedrooms (plans call this floor "wood")
    oakFloor: warm
      ? scanned('marble', 0.7, { rough: true, normal: false, color: '#efe6d8', roughness: 1, clearcoat: 0.3, clearcoatRoughness: 0.35 }) // pale terrazzo-like stone
      : scanned('porcelain', 2.4, { rough: true, color: '#e9e4db', roughness: 1, normalStrength: 0.5 }), // 120 cm matte tiles
    stoneFloor: scanned('porcelain', 1.2, { rough: true, roughness: 1, normalStrength: 0.8 }),
    bathFloor: scanned('granite_tile', 2.3, { rough: true, color: '#e4e1dc', roughness: 1 }),
    // glazed white wall tiles: bathrooms and the kitchen splashback
    tile: scanned('long_white_tiles', 1.27, { rough: true, color: '#fbf8f3', roughness: 1, clearcoat: 0.5, clearcoatRoughness: 0.2 }),
    deck: scanned('wood_floor_deck', 1.8, { rough: true, roughness: 1 }),
    glass: new THREE.MeshStandardMaterial({ color: '#e6eef0', roughness: 0.04, metalness: 0, transparent: true, opacity: 0.1, depthWrite: false }),
    frame: powder('#2a2725'),
    // wood
    // "walnut" is the flat's main joinery wood: light oak in Project 1, walnut in Project 2
    walnut: warm
      ? scanned('oak_veneer_01', 1.83, { rough: true, roughness: 0.9, clearcoat: 0.2, clearcoatRoughness: 0.4 })
      : scanned('walnut_veneer', 1.8, { rough: true, roughness: 0.9, clearcoat: 0.25, clearcoatRoughness: 0.4 }),
    slat: scanned('walnut_veneer', 1.2, { rough: true, color: warm ? '#e2c9a8' : '#c9a888', roughness: 1 }),
    oak: scanned('oak_veneer_01', 1.83, { rough: true, roughness: 1 }),
    trunk: scanned('oak_veneer_01', 0.3, { color: '#6d5847', roughness: 0.9 }),
    // painted (lacquered) kitchen doors in the flat's colour: the grain shows faintly through
    cabinet: scanned('white_oak_veneer', 0.5, { colorMap: false, color: c.cabinet, roughness: 0.42, normalStrength: 0.35, clearcoat: 0.35, clearcoatRoughness: 0.35 }),
    // fabric: the main upholstery, and the flat's signature armchair
    boucle: scanned('wool_boucle', 0.45, { colorMap: false, weave: true, normalStrength: 2, color: c.sofa, roughness: 1, ...fabric }),
    boucleShade: scanned('wool_boucle', 0.45, { colorMap: false, weave: true, normalStrength: 2, color: c.sofaShade, roughness: 1, ...fabric }),
    chairCloth: scanned('velour_velvet', 0.4, { colorMap: false, weave: true, rough: true, color: c.chair, roughness: 1, ...fabric, sheen: 0.5 }),
    shade: scanned('rough_linen', 0.2, { colorMap: false, weave: true, color: '#f3e6d2', roughness: 1, emissive: '#ffb36b', emissiveIntensity: 0.9 }),
    linen: scanned('rough_linen', 0.35, { colorMap: false, weave: true, color: '#f1ece3', roughness: 1, normalStrength: 1.6 }),
    duvet: scanned('waffle_pique_cotton', 0.4, { colorMap: false, weave: true, color: '#ece2d4', roughness: 1, normalStrength: 1.8 }),
    // cushions (two accent colours), the throws on sofa and beds, headboards
    oliveCloth: scanned('velour_velvet', 0.4, { colorMap: false, weave: true, rough: true, color: c.cushionA, roughness: 1, ...fabric, sheen: 0.6 }),
    taupe: scanned('velour_velvet', 0.4, { colorMap: false, weave: true, rough: true, color: c.cushionB, roughness: 1, ...fabric, sheen: 0.6 }),
    sage: scanned('ribbed_corduroy', 0.45, { colorMap: false, weave: true, color: c.headboard, roughness: 1, normalStrength: 1.8 }),
    knit: scanned('knitted_fleece', 0.4, { colorMap: false, weave: true, color: c.throw, roughness: 1, normalStrength: 1.3 }),
    rug: scanned('poly_wool_herringbone', 1.2, { colorMap: false, weave: true, color: c.rug, roughness: 1, normalStrength: 1.6 }),
    rugDark: scanned('poly_wool_herringbone', 1.2, { colorMap: false, weave: true, color: c.rugBorder, roughness: 1, normalStrength: 1.6 }),
    // stone, ceramic, metal
    marble: scanned('marble', 1.2, { rough: true, normal: false, roughness: 1, clearcoat: 0.6, clearcoatRoughness: 0.15 }),
    stonePot: scanned('clay_plaster', 0.6, { rough: true, color: '#ddd3c5', roughness: 1 }),
    terracotta: scanned('clay_plaster', 0.6, { rough: true, color: '#c38a6c', roughness: 1 }),
    ceramic: plain('#f5f3ef', 0.18, 0, { clearcoat: 0.8, clearcoatRoughness: 0.1 }),
    // vases and lamp bases: speckled glazed clay
    stoneware: scanned('stoneware', 0.25, { roughness: 0.35, normalStrength: 0.4, clearcoat: 0.6, clearcoatRoughness: 0.25 }),
    black: powder('#1f1d1b'),
    brass: scanned('brushed', 0.25, { colorMap: false, rough: true, color: '#c39a5e', roughness: 0.7, metalness: 1, normalStrength: 0.5 }),
    steel: metal('#cfd1d0', 0.7),
    screen: plain('#0b0b0c', 0.12, 0.2, { clearcoat: 1, clearcoatRoughness: 0.05 }),
    tv: new THREE.MeshStandardMaterial({ map: tvArt, emissiveMap: tvArt, emissive: '#ffffff', emissiveIntensity: 0.55, roughness: 0.35 }),
    mirror: plain('#e8ecec', 0.04, 1),
    // plants: real leaf shapes cut out of a small drawn texture
    leaf: new THREE.MeshStandardMaterial({ map: leafTex, color: '#8c9668', roughness: 0.7, alphaTest: 0.5, side: THREE.DoubleSide }),
    leafDark: new THREE.MeshStandardMaterial({ map: leafTex, color: '#6c7651', roughness: 0.7, alphaTest: 0.5, side: THREE.DoubleSide }),
    // art and books: canvas weave on the prints, cloth-bound covers on the books
    art1: scanned('rough_linen', 0.5, { colorMap: false, color: '#d8cbb6', roughness: 1, normalStrength: 0.5 }),
    art2: scanned('rough_linen', 0.5, { colorMap: false, color: '#9aa38c', roughness: 1, normalStrength: 0.5 }),
    art3: scanned('rough_linen', 0.5, { colorMap: false, color: c.art, roughness: 1, normalStrength: 0.5 }),
    art4: scanned('rough_linen', 0.5, { colorMap: false, color: '#ece4d6', roughness: 1, normalStrength: 0.5 }),
    book1: scanned('rough_linen', 0.2, { colorMap: false, color: '#7a5944', roughness: 0.9, normalStrength: 0.6 }),
    book2: scanned('rough_linen', 0.2, { colorMap: false, color: '#cdbca3', roughness: 0.9, normalStrength: 0.6 }),
    book3: scanned('rough_linen', 0.2, { colorMap: false, color: '#55605a', roughness: 0.9, normalStrength: 0.6 }),
    // light: the warm LED strips and lamp shades glow without a light source
    led: new THREE.MeshBasicMaterial({ color: '#ffb466', toneMapped: false }),
    lampGlow: new THREE.MeshBasicMaterial({ color: '#ffd29a', toneMapped: false }),
  }
  // Leaves keep their own texture coordinates (each leaf shows the whole drawing).
  mats.leaf.userData.foliage = true
  mats.leafDark.userData.foliage = true

  const dispose = () => {
    Object.values(mats).forEach((m) => m.dispose())
    cache.forEach((t) => t.dispose())
    tvArt.dispose()
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
