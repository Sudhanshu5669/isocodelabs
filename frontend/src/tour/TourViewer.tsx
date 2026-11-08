import { useEffect, useRef } from 'react'
import * as THREE from 'three'
import { OrbitControls } from 'three/addons/controls/OrbitControls.js'
import { EffectComposer } from 'three/addons/postprocessing/EffectComposer.js'
import { GTAOPass } from 'three/addons/postprocessing/GTAOPass.js'
import { OutputPass } from 'three/addons/postprocessing/OutputPass.js'
import { RenderPass } from 'three/addons/postprocessing/RenderPass.js'
import { RoomEnvironment } from 'three/addons/environments/RoomEnvironment.js'
import { buildFlat, type BuiltFlat, type FacingWall } from './buildFlat'
import { createMaterials } from './materials'
import type { Plan } from './plans'
import type { FlatTheme, LightMood, View } from './themes'

// The 3D viewer. This file and the 3D library are loaded only when someone
// opens a flat (React.lazy in the flat page), so the catalogue stays light.
//
// Three views:
//   flat  the whole flat as a diorama: near walls cut away, inner walls low
//   room  one room on its own, like an architect's cut-away model
//   walk  standing inside a room at eye height, looking around
//
// Kept fast on ordinary phones:
//   - frames are drawn only while something moves (no constant 60 fps loop)
//   - shadows are computed once and reused; the light never moves
//   - screen resolution is capped, lower on small or slower devices
//   - ambient occlusion (soft contact shadows, the main source of realism)
//     runs on laptops and desktops only, and only in the one-room and
//     walk-in views
//   - everything is freed when the viewer closes
//
// The light follows the way the flat faces (themes.ts), as in its photos:
//   morning  east: a low, golden sun, lamps nearly off
//   evening  west: a low amber sun and every lamp on
//   midday   south: a high, bright, white sun and short shadows
//   soft     north: no direct sun, an even sky light and soft shadows
// Lamps are real lights at each lamp (placed by furniture.ts); phones get the
// same palette without them, as they cost the most to draw. In the walk-in
// view the windows show the flat's own view: garden, rooftops or skyline.

export type TourView = { mode: 'flat' | 'room' | 'walk'; roomId: string | null }

type Props = {
  /** The flat's plan (planFor: mirrored for Towers B and D) and its look. */
  plan: Plan
  theme: FlatTheme
  view: TourView
  /** The visitor clicked a room in the diorama. */
  onPick: (roomId: string) => void
  /** The room under the pointer, for a hint ("Kitchen"). */
  onHover?: (roomId: string | null) => void
  /** 3D is not available on this device: show photos instead. */
  onFail: () => void
}

const DIORAMA_FOV = 26
/** Brightness of a lamp of strength 1 (candela). */
const LAMP = 7

/** How each time of day is lit. pos is where the sun stands, relative to the flat. */
const moods: Record<LightMood, {
  sun: string; power: number; pos: [number, number, number]; shadowRadius: number
  sky: string; ground: string; hemi: number; env: number; lamps: number; exposure: number
  skyStops: [number, string][]; haze: string
}> = {
  morning: {
    sun: '#ffd8aa', power: 2.6, pos: [-6, 5, 10], shadowRadius: 4, sky: '#eef3ff', ground: '#a89880', hemi: 0.45, env: 0.5, lamps: 0.2, exposure: 0.97,
    skyStops: [[0, '#a9c3e0'], [0.55, '#f3dcc2'], [1, '#f8cb9c']], haze: '#f1d6bd',
  },
  evening: {
    sun: '#ffbe85', power: 2.6, pos: [-10, 5, 6.5], shadowRadius: 4, sky: '#fff1e0', ground: '#a08870', hemi: 0.35, env: 0.5, lamps: 1, exposure: 0.95,
    skyStops: [[0, '#9fa7c4'], [0.45, '#e9b9a0'], [0.7, '#f7c98f'], [1, '#f2a974']], haze: '#eab48f',
  },
  midday: {
    sun: '#fff6ea', power: 3.1, pos: [-3, 14, 5], shadowRadius: 3, sky: '#f4f8ff', ground: '#b0a28c', hemi: 0.45, env: 0.55, lamps: 0, exposure: 0.98,
    skyStops: [[0, '#6fa3d8'], [0.6, '#bfd9ef'], [1, '#e6f0f6']], haze: '#d8e6ee',
  },
  soft: {
    sun: '#eef2ff', power: 1.1, pos: [-5, 12, 6], shadowRadius: 10, sky: '#f2f4f7', ground: '#a39a8c', hemi: 0.9, env: 0.65, lamps: 0.35, exposure: 1.04,
    skyStops: [[0, '#b9c3cc'], [0.6, '#dfe3e6'], [1, '#eceeee']], haze: '#dadfe2',
  },
}

/**
 * The sky outside the windows in the walk-in view, with the flat's own view
 * along the bottom: tree canopy (low floors), rooftops among trees (middle
 * floors) or a far, hazy city skyline (high floors).
 */
function windowView(mood: LightMood, view: View): THREE.CanvasTexture {
  const W = 512
  const H = 256
  const c = document.createElement('canvas')
  c.width = W
  c.height = H
  const g = c.getContext('2d')!
  const m = moods[mood]
  const grad = g.createLinearGradient(0, 0, 0, H)
  for (const [at, col] of m.skyStops) grad.addColorStop(at, col)
  g.fillStyle = grad
  g.fillRect(0, 0, W, H)

  let seed = 7
  const rand = () => ((seed = (seed * 16807) % 2147483647) / 2147483647)
  const leaf = mood === 'evening' ? '#6f6a40' : mood === 'soft' ? '#5f7258' : '#5c7a45'
  const trees = (top: number, size: number, col: string) => {
    g.fillStyle = col
    for (let x = -20; x < W + 20; x += size * 0.7) {
      const r = size * (0.6 + rand() * 0.5)
      g.beginPath()
      g.arc(x, top + rand() * size * 0.5, r, 0, Math.PI * 2)
      g.fill()
    }
    g.fillRect(0, top + size * 0.4, W, H)
  }
  if (view === 'garden') {
    trees(H * 0.42, 34, leaf)
    trees(H * 0.58, 26, mood === 'evening' ? '#5d5a36' : '#4d6a3a')
  } else if (view === 'rooftops') {
    g.fillStyle = m.haze
    g.fillRect(0, H * 0.6, W, H)
    for (let x = 0; x < W; ) {
      const w = 26 + rand() * 40
      const h = 14 + rand() * 30
      g.fillStyle = rand() > 0.5 ? '#cbbba5' : '#b9a891'
      g.fillRect(x, H * 0.66 - h, w - 3, h + H)
      x += w
    }
    trees(H * 0.72, 18, leaf)
  } else {
    // far towers in the haze, then the city fading towards the horizon
    for (let x = 0; x < W; ) {
      const w = 6 + rand() * 16
      const h = 4 + rand() * (rand() > 0.85 ? 40 : 16)
      g.fillStyle = mood === 'evening' ? '#b98f86' : mood === 'midday' ? '#9fb2c0' : '#aab3bb'
      g.globalAlpha = 0.75
      g.fillRect(x, H * 0.62 - h, w - 1, h)
      x += w + rand() * 4
    }
    g.globalAlpha = 1
    const ground = g.createLinearGradient(0, H * 0.62, 0, H)
    ground.addColorStop(0, m.haze)
    ground.addColorStop(1, mood === 'evening' ? '#b98f78' : '#a9ae9c')
    g.fillStyle = ground
    g.fillRect(0, H * 0.62, W, H)
  }
  const t = new THREE.CanvasTexture(c)
  t.colorSpace = THREE.SRGBColorSpace
  return t
}
const WALK_FOV = 68
const EYE = 1.55

export default function TourViewer({ plan, theme, view, onPick, onHover, onFail }: Props) {
  const hostRef = useRef<HTMLDivElement>(null)
  const apiRef = useRef<{ setView: (v: TourView, animate: boolean) => void } | null>(null)
  const viewRef = useRef(view)
  viewRef.current = view
  const cbRef = useRef({ onPick, onHover, onFail })
  cbRef.current = { onPick, onHover, onFail }

  useEffect(() => {
    const host = hostRef.current!
    const reducedMotion = window.matchMedia('(prefers-reduced-motion: reduce)').matches
    const nav = navigator as Navigator & { deviceMemory?: number }
    const lite = window.matchMedia('(max-width: 760px)').matches || (nav.hardwareConcurrency ?? 8) <= 4 || (nav.deviceMemory ?? 8) <= 4

    let renderer: THREE.WebGLRenderer
    try {
      renderer = new THREE.WebGLRenderer({ antialias: true, alpha: true, powerPreference: 'high-performance' })
    } catch {
      cbRef.current.onFail()
      return
    }
    renderer.setPixelRatio(Math.min(window.devicePixelRatio, lite ? 1.25 : 1.75))
    renderer.localClippingEnabled = true
    // Neutral tone mapping (the Khronos PBR standard) keeps colours true: warm
    // lamp light stays warm and the cream stage stays cream. (AgX, used
    // before, drained both towards grey.)
    const mood = moods[theme.light]
    // Mirror-image flats (Towers B and D) are viewed from the south-west, so
    // their back walls (where the TV and art hang) still face the camera.
    const side = theme.mirrored ? -1 : 1
    renderer.toneMapping = THREE.NeutralToneMapping
    renderer.toneMappingExposure = mood.exposure
    renderer.outputColorSpace = THREE.SRGBColorSpace
    renderer.shadowMap.enabled = true
    renderer.shadowMap.type = THREE.PCFSoftShadowMap
    renderer.shadowMap.autoUpdate = false // recomputed only when walls change
    renderer.domElement.className = 'tour-canvas'
    host.appendChild(renderer.domElement)

    const onLost = (e: Event) => {
      e.preventDefault()
      cbRef.current.onFail()
    }
    renderer.domElement.addEventListener('webglcontextlost', onLost)

    // ---- scene, light and the flat
    const scene = new THREE.Scene()
    // The stage colour, so the model sits on the page. Views drawn through the
    // effects chain (ambient occlusion) tone-map the background too, so they
    // get a brighter value that comes out as the same warm cream.
    const stage = new THREE.Color('#ece2d4')
    const stageThroughEffects = new THREE.Color().setRGB(1.08, 0.97, 0.84, THREE.LinearSRGBColorSpace)
    scene.background = stage
    // Seen through the windows in the walk-in view: the flat's sky and view.
    const outside = windowView(theme.light, theme.view)
    let redraw = () => {} // set once the render loop exists; textures call it when they arrive
    const pmrem = new THREE.PMREMGenerator(renderer)
    const envTex = pmrem.fromScene(new RoomEnvironment(), 0.04).texture
    scene.environment = envTex
    scene.environmentIntensity = mood.env // a neutral fill, so whites stay cream under the warm lights

    const { mats, dispose: disposeMats } = createMaterials({
      lite,
      anisotropy: Math.min(8, renderer.capabilities.getMaxAnisotropy()),
      onLoad: () => redraw(),
      theme,
    })
    const flat: BuiltFlat = buildFlat(plan, mats, { shadows: true })
    scene.add(flat.root)

    // The sun for this flat's time of day (mirrored with the flat).
    const sun = new THREE.DirectionalLight(mood.sun, mood.power)
    sun.position.set(flat.center.x + side * mood.pos[0], mood.pos[1], flat.center.z + mood.pos[2])
    sun.target.position.copy(flat.center)
    sun.castShadow = true
    sun.shadow.mapSize.set(lite ? 1024 : 2048, lite ? 1024 : 2048)
    const half = Math.max(flat.size.x, flat.size.z) * 0.75
    Object.assign(sun.shadow.camera, { left: -half, right: half, top: half, bottom: -half, near: 1, far: 40 })
    sun.shadow.bias = -0.0004
    sun.shadow.normalBias = 0.02
    sun.shadow.radius = mood.shadowRadius
    scene.add(sun, sun.target)
    scene.add(new THREE.HemisphereLight(mood.sky, mood.ground, lite ? mood.hemi + 0.25 : mood.hemi))
    // A warm ceiling light for the walk-in view, like a lit room in the
    // evening. It stays in the scene at zero brightness in the other views,
    // so switching views never rebuilds the shaders.
    const roomLight = new THREE.PointLight('#ffe7cc', 0, 9, 1.6)
    scene.add(roomLight)

    // Every lamp switched on: a warm light at each lamp anchor (desktops only).
    // They are created once; views change only their brightness, which keeps
    // the shaders as they are. Without shadows, so each is kept short-reaching.
    const lamps: { light: THREE.PointLight; roomId: string; base: number }[] = []
    if (!lite) {
      const all = [...flat.rooms.values()].flatMap((r) => r.lights.map((l) => ({ ...l, roomId: r.def.id })))
      all.sort((a, b) => b.strength - a.strength)
      for (const l of all.slice(0, 16)) {
        const light = new THREE.PointLight('#ffcf9e', 0, l.reach, 2)
        light.position.copy(l.pos)
        scene.add(light)
        lamps.push({ light, roomId: l.roomId, base: l.strength * LAMP * mood.lamps })
      }
    }

    // A soft shadow on the page under the diorama, as if it stood on the table.
    const ground = new THREE.Mesh(new THREE.PlaneGeometry(80, 80).rotateX(-Math.PI / 2), new THREE.ShadowMaterial({ opacity: 0.2 }))
    ground.position.set(flat.center.x, -0.33, flat.center.z)
    ground.receiveShadow = true
    scene.add(ground)

    const camera = new THREE.PerspectiveCamera(DIORAMA_FOV, 1, 0.05, 200)

    // Ambient occlusion: darkens corners, the floor under furniture and where
    // walls meet, which is what makes a render read as a photograph.
    let composer: EffectComposer | null = null
    let gtao: GTAOPass | null = null
    if (!lite) {
      composer = new EffectComposer(renderer)
      composer.addPass(new RenderPass(scene, camera))
      gtao = new GTAOPass(scene, camera, 1, 1)
      gtao.updateGtaoMaterial({ radius: 0.32, distanceExponent: 1.6, thickness: 1.2, scale: 1.15, samples: 16 })
      gtao.updatePdMaterial({ lumaPhi: 10, depthPhi: 2, normalPhi: 3, radius: 5, rings: 2, samples: 12 })
      gtao.blendIntensity = 0.95
      composer.addPass(gtao)
      composer.addPass(new OutputPass())
    }
    const controls = new OrbitControls(camera, renderer.domElement)
    controls.enableDamping = true
    controls.dampingFactor = 0.09
    controls.enablePan = false

    // ---- drawing on demand
    let frame = 0
    let visible = true
    const invalidate = () => {
      if (!frame && visible) frame = requestAnimationFrame(tick)
    }

    let anim: null | {
      t0: number
      dur: number
      fromPos: THREE.Vector3
      toPos: THREE.Vector3
      fromTarget: THREE.Vector3
      toTarget: THREE.Vector3
      fromFov: number
      toFov: number
      done: () => void
    } = null
    const ease = (t: number) => (t < 0.5 ? 4 * t * t * t : 1 - Math.pow(-2 * t + 2, 3) / 2)

    let mode: TourView['mode'] = 'flat'
    let roomId: string | null = null

    // Hide the walls between the camera and the room, keep the far ones.
    const facing = (w: FacingWall) => {
      if (w.low) return true
      const toCam = new THREE.Vector3().subVectors(camera.position, w.center)
      return w.normal.dot(toCam) <= 0.05
    }
    const lastVis = new Map<THREE.Object3D, boolean>()
    const setVis = (o: THREE.Object3D, v: boolean) => {
      o.visible = v
      if (lastVis.get(o) !== v) {
        lastVis.set(o, v)
        renderer.shadowMap.needsUpdate = true
      }
    }
    const updateFacing = () => {
      if (mode === 'flat') flat.exteriorWalls.forEach((w) => setVis(w.group, facing(w)))
      if (mode === 'room' && roomId) flat.rooms.get(roomId)?.walls.forEach((w) => setVis(w.group, facing(w)))
    }

    function tick(now: number) {
      frame = 0
      let moving = false
      if (anim) {
        const t = Math.min(1, (now - anim.t0) / anim.dur)
        const k = ease(t)
        camera.position.lerpVectors(anim.fromPos, anim.toPos, k)
        controls.target.lerpVectors(anim.fromTarget, anim.toTarget, k)
        camera.fov = anim.fromFov + (anim.toFov - anim.fromFov) * k
        camera.updateProjectionMatrix()
        camera.lookAt(controls.target)
        if (t >= 1) {
          const done = anim.done
          anim = null
          done()
        } else moving = true
      } else {
        moving = controls.update()
      }
      updateFacing()
      // The whole-home view cuts walls with a clipping plane, which the AO
      // pass cannot see, so that view is drawn without it.
      if (composer && mode !== 'flat') composer.render()
      else renderer.render(scene, camera)
      if (moving) invalidate()
    }
    redraw = invalidate
    controls.addEventListener('change', invalidate)

    // ---- the three views
    const dioramaPose = (target: THREE.Vector3, span: number, aspect: number, room = false) => {
      const fit = span / (2 * Math.tan(THREE.MathUtils.degToRad(DIORAMA_FOV / 2)))
      const dist = fit * (aspect < 1 ? 1.3 / Math.max(aspect, 0.55) : 1.14) * (room ? 1.22 : 1)
      const dir = new THREE.Vector3(0.82 * side, 0.95, 1).normalize()
      return { pos: target.clone().addScaledVector(dir, dist), target, dist }
    }

    function setView(v: TourView, animate: boolean) {
      const aspect = camera.aspect
      const room = v.roomId ? flat.rooms.get(v.roomId) : undefined
      const nextMode = v.mode !== 'flat' && !room ? 'flat' : v.mode
      let toPos: THREE.Vector3
      let toTarget: THREE.Vector3
      let toFov = DIORAMA_FOV

      // What is shown in each view.
      const showRoomOnly = nextMode === 'room'
      flat.rooms.forEach((r, id) => {
        r.content.visible = !showRoomOnly || id === v.roomId
        r.slab.visible = showRoomOnly && id === v.roomId
        r.walls.forEach((w) => setVis(w.group, false))
      })
      flat.flatSlab.visible = nextMode !== 'room'
      setVis(flat.interiorWalls, nextMode !== 'room')
      flat.exteriorWalls.forEach((w) => setVis(w.group, nextMode !== 'room')) // flat view then hides the near ones
      flat.ceiling.visible = nextMode === 'walk'
      roomLight.intensity = nextMode === 'walk' && room ? 3 : 0
      // Lamps: all on in the whole-home view; in one room, its own lamps
      // (with a little spill from the others when walking through).
      for (const l of lamps) {
        const own = l.roomId === v.roomId
        l.light.intensity = nextMode === 'flat' ? l.base * 0.7 : own ? l.base : nextMode === 'walk' ? l.base * 0.35 : 0
      }
      scene.background = nextMode === 'walk' ? outside : composer && nextMode !== 'flat' ? stageThroughEffects : stage
      if (room) roomLight.position.set(room.center.x, 2.35, room.center.z)
      flat.cut.constant = nextMode === 'walk' ? 100 : 1.15
      renderer.shadowMap.needsUpdate = true

      if (nextMode === 'walk' && room) {
        const [ex, ez] = room.def.eye
        const [lx, lz] = room.def.look
        toPos = new THREE.Vector3(ex, EYE, ez)
        const dir = new THREE.Vector3(lx - ex, 0, lz - ez).normalize()
        dir.y = -0.12
        toTarget = toPos.clone().addScaledVector(dir.normalize(), 0.05)
        toFov = WALK_FOV
      } else if (nextMode === 'room' && room) {
        const r = room.def
        const span = Math.hypot(r.x1 - r.x0, r.z1 - r.z0) * 0.95 + 0.6
        const p = dioramaPose(room.center.clone().setY(0.55), span, aspect, true)
        toPos = p.pos
        toTarget = p.target
      } else {
        const span = Math.hypot(flat.size.x, flat.size.z) * 0.92
        const p = dioramaPose(flat.center.clone().setY(0.3), span, aspect)
        toPos = p.pos
        toTarget = p.target
      }

      const finish = () => {
        mode = nextMode
        roomId = v.roomId
        const walk = nextMode === 'walk'
        controls.enabled = true
        controls.enableZoom = !walk
        controls.rotateSpeed = walk ? -0.32 : 0.7 // inside: drag the view, like a photo sphere
        if (walk) {
          controls.minDistance = 0.01
          controls.maxDistance = 0.2
          controls.minPolarAngle = Math.PI / 2 - 0.55
          controls.maxPolarAngle = Math.PI / 2 + 0.5
        } else {
          const dist = camera.position.distanceTo(controls.target)
          controls.minDistance = dist * 0.55
          controls.maxDistance = dist * 1.6
          controls.minPolarAngle = 0.35
          controls.maxPolarAngle = 1.2
        }
        // Dioramas turn within the south-east quarter (south-west for a
        // mirror-image flat), so the back walls, where the TV and art hang,
        // are never cut away.
        controls.minAzimuthAngle = walk ? -Infinity : side > 0 ? 0.12 : -1.45
        controls.maxAzimuthAngle = walk ? Infinity : side > 0 ? 1.45 : -0.12
        updateFacing()
        renderer.shadowMap.needsUpdate = true
        invalidate()
      }

      mode = nextMode // walls follow the new view while the camera flies
      roomId = v.roomId
      controls.enabled = false
      if (!animate || reducedMotion) {
        camera.position.copy(toPos)
        controls.target.copy(toTarget)
        camera.fov = toFov
        camera.updateProjectionMatrix()
        camera.lookAt(toTarget)
        finish()
      } else {
        anim = {
          t0: performance.now(),
          dur: 950,
          fromPos: camera.position.clone(),
          toPos,
          fromTarget: controls.target.clone(),
          toTarget,
          fromFov: camera.fov,
          toFov,
          done: finish,
        }
        invalidate()
      }
    }
    apiRef.current = { setView }

    // ---- size, visibility, pointer
    const resize = () => {
      const w = host.clientWidth
      const h = host.clientHeight
      if (!w || !h) return
      renderer.setSize(w, h, false)
      composer?.setPixelRatio(renderer.getPixelRatio())
      composer?.setSize(w, h)
      camera.aspect = w / h
      camera.updateProjectionMatrix()
      invalidate()
    }
    const ro = new ResizeObserver(resize)
    ro.observe(host)
    resize()

    const io = new IntersectionObserver(([entry]) => {
      visible = entry.isIntersecting
      if (visible) invalidate()
    })
    io.observe(host)

    const ray = new THREE.Raycaster()
    const ndc = new THREE.Vector2()
    const picks = [...flat.rooms.values()].map((r) => r.pick)
    const roomUnder = (e: PointerEvent): string | null => {
      if (mode !== 'flat') return null
      const rect = renderer.domElement.getBoundingClientRect()
      ndc.set(((e.clientX - rect.left) / rect.width) * 2 - 1, -((e.clientY - rect.top) / rect.height) * 2 + 1)
      ray.setFromCamera(ndc, camera)
      const hit = ray.intersectObjects(picks, false)[0]
      return hit ? (hit.object.userData.roomId as string) : null
    }
    let down: { x: number; y: number } | null = null
    let hovered: string | null = null
    const onDown = (e: PointerEvent) => (down = { x: e.clientX, y: e.clientY })
    const onUp = (e: PointerEvent) => {
      if (down && Math.hypot(e.clientX - down.x, e.clientY - down.y) < 6) {
        const id = roomUnder(e)
        if (id) cbRef.current.onPick(id)
      }
      down = null
    }
    const onMove = (e: PointerEvent) => {
      if (e.buttons) return
      const id = roomUnder(e)
      if (id !== hovered) {
        hovered = id
        renderer.domElement.style.cursor = id ? 'pointer' : ''
        cbRef.current.onHover?.(id)
      }
    }
    const el = renderer.domElement
    el.addEventListener('pointerdown', onDown)
    el.addEventListener('pointerup', onUp)
    el.addEventListener('pointermove', onMove)

    setView(viewRef.current, false)
    renderer.shadowMap.needsUpdate = true
    invalidate()

    return () => {
      cancelAnimationFrame(frame)
      apiRef.current = null
      ro.disconnect()
      io.disconnect()
      el.removeEventListener('pointerdown', onDown)
      el.removeEventListener('pointerup', onUp)
      el.removeEventListener('pointermove', onMove)
      el.removeEventListener('webglcontextlost', onLost)
      controls.dispose()
      gtao?.dispose()
      composer?.dispose()
      flat.dispose()
      disposeMats()
      ground.geometry.dispose()
      ;(ground.material as THREE.Material).dispose()
      envTex.dispose()
      outside.dispose()
      pmrem.dispose()
      renderer.dispose()
      el.remove()
    }
  }, [plan, theme])

  // Changing rooms or views flies the camera there.
  useEffect(() => {
    apiRef.current?.setView(view, true)
  }, [view.mode, view.roomId]) // eslint-disable-line react-hooks/exhaustive-deps

  return <div ref={hostRef} className="tour-host" />
}

