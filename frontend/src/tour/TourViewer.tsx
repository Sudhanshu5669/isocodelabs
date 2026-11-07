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
import { plans, type Layout } from './plans'

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
// The light is a warm early evening: a low golden sun through the windows,
// and every lamp in the flat switched on (a real light at each lamp, placed
// by furniture.ts). Phones get the same warm palette without the lamp
// lights, which cost the most to draw.

export type TourView = { mode: 'flat' | 'room' | 'walk'; roomId: string | null }

type Props = {
  layout: Layout
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

/** A soft dusk gradient for the sky outside the windows. */
function duskSky(): THREE.CanvasTexture {
  const c = document.createElement('canvas')
  c.width = 2
  c.height = 256
  const g = c.getContext('2d')!
  const grad = g.createLinearGradient(0, 0, 0, 256)
  grad.addColorStop(0, '#9fa7c4')
  grad.addColorStop(0.45, '#e9b9a0')
  grad.addColorStop(0.7, '#f7c98f')
  grad.addColorStop(1, '#f2a974')
  g.fillStyle = grad
  g.fillRect(0, 0, 2, 256)
  const t = new THREE.CanvasTexture(c)
  t.colorSpace = THREE.SRGBColorSpace
  return t
}
const WALK_FOV = 68
const EYE = 1.55

export default function TourViewer({ layout, view, onPick, onHover, onFail }: Props) {
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
    renderer.toneMapping = THREE.NeutralToneMapping
    renderer.toneMappingExposure = 0.95
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
    // Seen through the windows in the walk-in view: a warm dusk sky.
    const dusk = duskSky()
    let redraw = () => {} // set once the render loop exists; textures call it when they arrive
    const pmrem = new THREE.PMREMGenerator(renderer)
    const envTex = pmrem.fromScene(new RoomEnvironment(), 0.04).texture
    scene.environment = envTex
    scene.environmentIntensity = 0.5 // a neutral fill, so whites stay cream under the warm lights

    const { mats, dispose: disposeMats } = createMaterials({
      lite,
      anisotropy: Math.min(8, renderer.capabilities.getMaxAnisotropy()),
      onLoad: () => redraw(),
    })
    const plan = plans[layout]
    const flat: BuiltFlat = buildFlat(plan, mats, { shadows: true })
    scene.add(flat.root)

    // Golden hour: a low, deep-amber sun that lays long warm patches through the glass.
    const sun = new THREE.DirectionalLight('#ffd0a0', 2.7)
    sun.position.set(flat.center.x - 9, 8.5, flat.center.z + 7)
    sun.target.position.copy(flat.center)
    sun.castShadow = true
    sun.shadow.mapSize.set(lite ? 1024 : 2048, lite ? 1024 : 2048)
    const half = Math.max(flat.size.x, flat.size.z) * 0.75
    Object.assign(sun.shadow.camera, { left: -half, right: half, top: half, bottom: -half, near: 1, far: 40 })
    sun.shadow.bias = -0.0004
    sun.shadow.normalBias = 0.02
    sun.shadow.radius = 4
    scene.add(sun, sun.target)
    scene.add(new THREE.HemisphereLight('#fff1e0', '#a08870', lite ? 0.6 : 0.35))
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
        lamps.push({ light, roomId: l.roomId, base: l.strength * LAMP })
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
      const dir = new THREE.Vector3(0.82, 0.95, 1).normalize()
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
      scene.background = nextMode === 'walk' ? dusk : composer && nextMode !== 'flat' ? stageThroughEffects : stage
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
        // Dioramas turn within the south-east quarter, so the north and west
        // walls are always the back walls (art hangs there) and never cut away.
        controls.minAzimuthAngle = walk ? -Infinity : 0.12
        controls.maxAzimuthAngle = walk ? Infinity : 1.45
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
      dusk.dispose()
      pmrem.dispose()
      renderer.dispose()
      el.remove()
    }
  }, [layout])

  // Changing rooms or views flies the camera there.
  useEffect(() => {
    apiRef.current?.setView(view, true)
  }, [view.mode, view.roomId]) // eslint-disable-line react-hooks/exhaustive-deps

  return <div ref={hostRef} className="tour-host" />
}

