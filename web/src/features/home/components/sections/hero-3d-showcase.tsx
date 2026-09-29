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
import { motion, useMotionValue, useReducedMotion, useSpring } from 'motion/react'
import { useCallback, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'

/**
 * 首页 3D 能力展示（纯 CSS 3D + 指针视差，零 WebGL 依赖）
 *
 * 迭代说明（v1.3.60 第 2 版）：初版三层等高卡片重叠 34px，视觉上像「糊在一起
 * 的卡片堆」且与上方终端演示区争抢空间。改为**层叠式扇面**：
 *   - 每层高度更矮（88px）、错位更明确（56px），形成可辨识的 3D 扇面
 *   - 层间用 scale + 透明度区分远近（近层更实、远层更淡），强化纵深
 *   - 整体高度收敛，避免与终端区视觉打架
 *
 * 无障碍：`prefers-reduced-motion: reduce` 时关闭倾斜/浮动，保留静态分层。
 * 该组件整体 `aria-hidden`（纯装饰），不影响读屏与键盘导航。
 */
export function Hero3DShowcase() {
  const { t } = useTranslation()
  const shouldReduce = useReducedMotion()
  const containerRef = useRef<HTMLDivElement>(null)
  const [isHovering, setIsHovering] = useState(false)

  const rotateX = useSpring(useMotionValue(0), { stiffness: 140, damping: 18 })
  const rotateY = useSpring(useMotionValue(0), { stiffness: 140, damping: 18 })

  const handlePointerMove = useCallback(
    (event: React.PointerEvent<HTMLDivElement>) => {
      if (shouldReduce || !containerRef.current) {return}
      const rect = containerRef.current.getBoundingClientRect()
      const px = (event.clientX - rect.left) / rect.width - 0.5
      const py = (event.clientY - rect.top) / rect.height - 0.5
      // 限制在 ±9°：保证层内文字始终可读
      rotateY.set(px * 18)
      rotateX.set(-py * 18)
    },
    [rotateX, rotateY, shouldReduce]
  )

  const handlePointerLeave = useCallback(() => {
    setIsHovering(false)
    rotateX.set(0)
    rotateY.set(0)
  }, [rotateX, rotateY])

  // 由近及远：深度递减、透明度递减、缩放置信递减 —— 形成清晰纵深
  const layers = [
    {
      key: 'routing',
      label: t('Smart Routing'),
      hint: t('Route every request to the healthiest upstream'),
      depth: 0,
      inset: 0,
      opacity: 1,
      tone: 'border-emerald-500/30 bg-emerald-500/[0.07]',
      dot: 'bg-emerald-500',
    },
    {
      key: 'billing',
      label: t('Transparent Billing'),
      hint: t('Usage and cost visible before you commit'),
      depth: 52,
      inset: 22,
      opacity: 0.78,
      tone: 'border-violet-500/25 bg-violet-500/[0.06]',
      dot: 'bg-violet-500',
    },
    {
      key: 'protocol',
      label: t('Unified Protocol'),
      hint: t('One endpoint for every provider and SDK'),
      depth: 104,
      inset: 44,
      opacity: 0.55,
      tone: 'border-blue-500/25 bg-blue-500/[0.06]',
      dot: 'bg-blue-500',
    },
  ]

  return (
    <div
      ref={containerRef}
      aria-hidden
      className='relative hidden w-full max-w-md select-none sm:block'
      style={{ perspective: '1100px' }}
      onPointerMove={handlePointerMove}
      onPointerEnter={() => setIsHovering(true)}
      onPointerLeave={handlePointerLeave}
    >
      <motion.div
        className='relative'
        style={{
          rotateX: shouldReduce ? 0 : rotateX,
          rotateY: shouldReduce ? 0 : rotateY,
          transformStyle: 'preserve-3d',
        }}
      >
        {/* 高度 = 最远层的起始偏移 + 单层高，保证不超出容器 */}
        <div className='relative' style={{ height: '196px' }}>
          {layers.map((layer, index) => (
            <motion.div
              key={layer.key}
              className={`absolute rounded-xl border backdrop-blur-sm ${layer.tone}`}
              style={{
                inset: `0 ${layer.inset}px auto ${layer.inset}px`,
                top: `${layer.depth}px`,
                height: '86px',
                zIndex: layers.length - index,
                boxShadow: isHovering
                  ? '0 14px 36px -14px oklch(0.2 0.02 250 / 0.4)'
                  : '0 8px 24px -14px oklch(0.2 0.02 250 / 0.28)',
              }}
              // 默认可见（不做 initial 隐藏）：即使 JS/IntersectionObserver
              // 未执行或截图工具抓取整页，内容也不会永久消失——这是可用性底线，
              // 动画只作为「锦上添花」的入场增强。
              initial={false}
              animate={
                shouldReduce
                  ? { opacity: layer.opacity }
                  : { opacity: layer.opacity, y: 0, scale: 1 }
              }
              transition={{
                duration: 0.5,
                delay: index * 0.08,
                ease: [0.16, 1, 0.3, 1],
              }}
              whileHover={
                shouldReduce ? undefined : { translateZ: layer.depth + 12 }
              }
            >
              {/* 顶部对齐：被上层覆盖的仅是卡片下半部空白，标签与说明始终可读 */}
              <div className='flex h-full flex-col justify-start gap-1 px-4 pt-3'>
                <div className='flex items-center gap-2'>
                  <span className={`size-1.5 shrink-0 rounded-full ${layer.dot}`} />
                  <span className='text-foreground/85 text-xs font-medium tracking-wide'>
                    {layer.label}
                  </span>
                </div>
                <span className='text-muted-foreground/70 line-clamp-1 text-[11px] leading-snug'>
                  {layer.hint}
                </span>
              </div>
            </motion.div>
          ))}
        </div>
      </motion.div>
    </div>
  )
}
