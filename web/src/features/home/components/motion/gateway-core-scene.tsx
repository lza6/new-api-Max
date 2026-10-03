/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import { Canvas, useFrame } from '@react-three/fiber'
import { useEffect, useMemo, useRef, type ReactNode } from 'react'
import {
  AdditiveBlending,
  type Group,
  BufferGeometry,
  Float32BufferAttribute,
} from 'three'

/**
 * Hero "gateway core" scene: a wireframe icosahedron (the gateway) with satellite
 * nodes (upstream providers) tethered by hairlines. Pure three primitives — no
 * drei, no textures, no environment maps — so the lazy chunk stays small and the
 * render is deterministic across devices.
 *
 * Decorative only: the host wrapper marks the canvas `aria-hidden`, disables it
 * under `prefers-reduced-motion`, and never relies on it for information.
 */

const ACCENTS = [
  '#fbbf24', // amber
  '#fb7185', // rose
  '#a78bfa', // violet
  '#34d399', // emerald
  '#60a5fa', // blue
  '#f472b6', // pink
]

const NODE_COUNT = ACCENTS.length
const RADIUS = 1.55

interface NodeRingProps {
  radius: number
  count: number
}

function NodeRing(props: NodeRingProps) {
  const groupRef = useRef<Group>(null)

  // Static geometry: one sphere per provider node and one hairline per tether,
  // both built once. Rebuilding them per frame would defeat the point of a
  // declarative scene.
  const nodes = useMemo(() => {
    return Array.from({ length: props.count }, (_, i) => {
      const angle = (i / props.count) * Math.PI * 2
      const wobble = i % 2 === 0 ? 0.35 : -0.35
      return {
        key: `node-${i}`,
        position: [
          Math.cos(angle) * props.radius,
          wobble,
          Math.sin(angle) * props.radius,
        ] as [number, number, number],
        color: ACCENTS[i % ACCENTS.length],
      }
    })
  }, [props.count, props.radius])

  const tetherGeometry = useMemo(() => {
    const positions: number[] = []
    for (const node of nodes) {
      positions.push(0, 0, 0, ...node.position)
    }
    const geometry = new BufferGeometry()
    geometry.setAttribute('position', new Float32BufferAttribute(positions, 3))
    return geometry
  }, [nodes])

  // r3f auto-disposes geometry it creates from JSX args, but a BufferGeometry
  // handed to <primitive object> is our own allocation and would otherwise leak
  // on unmount (the GPU buffer is freed with the renderer, but the CPU-side
  // attribute arrays would linger until GC).
  useEffect(() => () => tetherGeometry.dispose(), [tetherGeometry])

  useFrame((state) => {
    const group = groupRef.current
    if (!group) {return}
    const t = state.clock.elapsedTime
    group.rotation.y = t * 0.16
    // Gentle pointer parallax; the camera itself stays fixed so text never moves
    // relative to the canvas frame.
    group.rotation.x = state.pointer.y * 0.18
    group.rotation.z = state.pointer.x * 0.06
  })

  return (
    <group ref={groupRef}>
      <lineSegments>
        <primitive object={tetherGeometry} attach='geometry' />
        <lineBasicMaterial
          color='#94a3b8'
          transparent
          opacity={0.28}
          blending={AdditiveBlending}
        />
      </lineSegments>
      {nodes.map((node) => (
        <mesh key={node.key} position={node.position}>
          <sphereGeometry args={[0.085, 20, 20]} />
          <meshStandardMaterial
            color={node.color}
            emissive={node.color}
            emissiveIntensity={0.9}
            roughness={0.35}
            metalness={0.1}
          />
        </mesh>
      ))}
    </group>
  )
}

function Core() {
  const meshRef = useRef<Group>(null)

  useFrame((state) => {
    const group = meshRef.current
    if (!group) {return}
    const t = state.clock.elapsedTime
    group.rotation.y = -t * 0.22
    group.rotation.x = t * 0.08
  })

  return (
    <group ref={meshRef}>
      <mesh>
        <icosahedronGeometry args={[1.05, 1]} />
        <meshBasicMaterial color='#6366f1' wireframe transparent opacity={0.42} />
      </mesh>
      <mesh>
        <icosahedronGeometry args={[0.52, 0]} />
        <meshStandardMaterial
          color='#312e81'
          emissive='#6366f1'
          emissiveIntensity={0.7}
          roughness={0.25}
          metalness={0.6}
          transparent
          opacity={0.92}
        />
      </mesh>
    </group>
  )
}

function Scene() {
  return (
    <>
      <ambientLight intensity={0.7} />
      <pointLight position={[4, 4, 4]} intensity={60} color='#fbbf24' />
      <pointLight position={[-4, -2, -2]} intensity={45} color='#8b5cf6' />
      <Core />
      <NodeRing radius={RADIUS} count={NODE_COUNT} />
    </>
  )
}

interface GatewayCoreSceneProps {
  children?: ReactNode
}

/**
 * Canvas host. `dpr` is capped and antialiasing kept on for clean hairlines;
 * the canvas is transparent so the page background/gradients show through.
 */
export default function GatewayCoreScene(_props: GatewayCoreSceneProps) {
  return (
    <Canvas
      dpr={[1, 1.6]}
      gl={{ antialias: true, alpha: true, powerPreference: 'high-performance' }}
      camera={{ position: [0, 0, 5], fov: 42 }}
      style={{ width: '100%', height: '100%' }}
    >
      <Scene />
    </Canvas>
  )
}
