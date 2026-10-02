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
import { useReducedMotion } from 'motion/react'
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { AnimateInView } from '@/components/animate-in-view'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { cn } from '@/lib/utils'

/** Accent tone for the selected endpoint; class strings stay static so they
 *  survive Tailwind's content scan. Values are semantic, not hardcoded hues. */
type FlowTone = 'emerald' | 'amber' | 'blue' | 'violet'

interface EndpointDemo {
  id: string
  label: string
  path: string
  upstream: string
  tone: FlowTone
  /** Milliseconds spent establishing the upstream connection. */
  connect: number
  /** Milliseconds spent uploading the request body to the upstream. */
  upload: number
  /** Milliseconds spent waiting on upstream prefill / first token. */
  compute: number
}

const TONE_CLASSES: Record<
  FlowTone,
  { activeDot: string; activeText: string; activeRing: string; segment: string }
> = {
  emerald: {
    activeDot: 'bg-emerald-500',
    activeText: 'text-emerald-600 dark:text-emerald-400',
    activeRing: 'border-emerald-500/40 bg-emerald-500/[0.07]',
    segment: 'bg-emerald-500/70',
  },
  amber: {
    activeDot: 'bg-amber-500',
    activeText: 'text-amber-600 dark:text-amber-400',
    activeRing: 'border-amber-500/40 bg-amber-500/[0.07]',
    segment: 'bg-amber-500/70',
  },
  blue: {
    activeDot: 'bg-blue-500',
    activeText: 'text-blue-600 dark:text-blue-400',
    activeRing: 'border-blue-500/40 bg-blue-500/[0.07]',
    segment: 'bg-blue-500/70',
  },
  violet: {
    activeDot: 'bg-violet-500',
    activeText: 'text-violet-600 dark:text-violet-400',
    activeRing: 'border-violet-500/40 bg-violet-500/[0.07]',
    segment: 'bg-violet-500/70',
  },
}

// Illustrative figures, shaped after observed production latency: most of the
// first-token delay lives in upstream prefill, not in transport. Kept static
// on purpose so the section is a stable narrative, not a live metric.
const ENDPOINTS: EndpointDemo[] = [
  {
    id: 'chat',
    label: 'Chat',
    path: '/v1/chat/completions',
    upstream: 'OpenAI-compatible',
    tone: 'emerald',
    connect: 6,
    upload: 692,
    compute: 5874,
  },
  {
    id: 'responses',
    label: 'Responses',
    path: '/v1/responses',
    upstream: 'OpenAI-compatible',
    tone: 'amber',
    connect: 8,
    upload: 431,
    compute: 2980,
  },
  {
    id: 'claude',
    label: 'Claude',
    path: '/v1/messages',
    upstream: 'Anthropic',
    tone: 'blue',
    connect: 12,
    upload: 507,
    compute: 4105,
  },
  {
    id: 'gemini',
    label: 'Gemini',
    path: '/v1beta/models/{model}',
    upstream: 'Google',
    tone: 'violet',
    connect: 9,
    upload: 288,
    compute: 2140,
  },
]

const STAGE_COUNT = 4
const STAGE_INTERVAL_MS = 460

interface FlowStage {
  label: string
  hint: string
}

function FlowDiagram(props: { demo: EndpointDemo }) {
  const { t } = useTranslation()
  const shouldReduce = useReducedMotion()
  const [stage, setStage] = useState(shouldReduce ? STAGE_COUNT : 0)

  useEffect(() => {
    if (shouldReduce) {
      setStage(STAGE_COUNT)
      return
    }
    setStage(0)
    const timers = Array.from({ length: STAGE_COUNT }, (_, index) =>
      window.setTimeout(() => setStage(index + 1), (index + 1) * STAGE_INTERVAL_MS)
    )
    return () => {
      timers.forEach((timer) => window.clearTimeout(timer))
    }
  }, [shouldReduce])

  const { demo } = props
  const tone = TONE_CLASSES[demo.tone]
  const total = demo.connect + demo.upload + demo.compute
  const width = (value: number) => `${(value / total) * 100}%`

  const stages: FlowStage[] = [
    { label: t('Client'), hint: demo.path },
    { label: t('Gateway'), hint: t('Auth, routing, billing') },
    { label: t('Upstream'), hint: demo.upstream },
    { label: t('Response'), hint: t('Token stream back') },
  ]

  const segments: { label: string; value: number; className: string }[] = [
    { label: t('Connect'), value: demo.connect, className: 'bg-muted-foreground/40' },
    { label: t('Upload'), value: demo.upload, className: tone.segment },
    { label: t('Upstream compute'), value: demo.compute, className: 'bg-primary/70' },
  ]

  return (
    <div className='grid gap-10 lg:grid-cols-12 lg:gap-12'>
      {/* Pipeline */}
      <div className='lg:col-span-7'>
        <div className='relative grid grid-cols-2 gap-x-4 gap-y-6 sm:grid-cols-4 sm:gap-x-3'>
          {/* Hairline rail behind the stage nodes (desktop only). */}
          <div
            aria-hidden
            className='pointer-events-none absolute top-5 right-[12%] left-[12%] hidden h-px bg-border/60 sm:block'
          />
          {stages.map((item, index) => {
            const reached = index < stage
            return (
              <div key={item.label} className='relative flex flex-col items-center gap-2.5 text-center'>
                <span
                  className={cn(
                    'flex size-10 items-center justify-center rounded-full border text-xs font-semibold tabular-nums transition-all duration-500',
                    reached
                      ? `${tone.activeRing} ${tone.activeText}`
                      : 'border-border/50 text-muted-foreground/50'
                  )}
                >
                  {index + 1}
                </span>
                <span
                  className={cn(
                    'text-xs font-medium tracking-wide transition-colors duration-500',
                    reached ? 'text-foreground' : 'text-muted-foreground/60'
                  )}
                >
                  {item.label}
                </span>
                <span className='text-muted-foreground/50 -mt-1 font-mono text-[10px] leading-snug'>
                  {item.hint}
                </span>
              </div>
            )
          })}
        </div>
      </div>

      {/* Latency breakdown */}
      <div className='lg:col-span-5'>
        <div className='border-border/50 bg-card/40 rounded-2xl border p-5 backdrop-blur-xs'>
          <div className='mb-4 flex items-baseline justify-between'>
            <span className='text-muted-foreground/70 text-[11px] font-medium tracking-[0.22em] uppercase'>
              {t('Latency')}
            </span>
            <span className='text-foreground/85 text-sm tabular-nums'>
              {total.toLocaleString()}
              <span className='text-muted-foreground/60 ml-1 text-[11px] tracking-wider uppercase'>
                ms
              </span>
            </span>
          </div>

          <div className='flex h-2 w-full overflow-hidden rounded-full bg-muted/50'>
            {segments.map((segment) => (
              <span
                key={segment.label}
                className={cn('h-full min-w-[3px]', segment.className)}
                style={{ width: width(segment.value) }}
              />
            ))}
          </div>

          <ul className='mt-4 space-y-2'>
            {segments.map((segment) => (
              <li key={segment.label} className='flex items-center gap-2.5 text-xs'>
                <span className={cn('size-2 shrink-0 rounded-full', segment.className)} />
                <span className='text-muted-foreground/80 flex-1'>{segment.label}</span>
                <span className='text-foreground/70 tabular-nums'>{segment.value}</span>
              </li>
            ))}
          </ul>
        </div>
      </div>
    </div>
  )
}

interface RequestFlowProps {
  className?: string
}

/**
 * Interactive request-routing showcase: pick an endpoint, watch the request
 * travel client → gateway → upstream → response, and read where the latency
 * actually goes. Animation honours `prefers-reduced-motion`.
 */
export function RequestFlow(props: RequestFlowProps) {
  const { t } = useTranslation()
  const [activeId, setActiveId] = useState(ENDPOINTS[0].id)

  return (
    <section
      className={cn('relative z-10 px-6 py-20 md:py-24', props.className)}
    >
      <div className='mx-auto max-w-6xl'>
        <AnimateInView className='mb-12 max-w-xl'>
          <p className='text-muted-foreground/70 mb-4 text-[11px] font-medium tracking-[0.28em] uppercase'>
            {t('Request Flow')}
          </p>
          <h2 className='font-serif text-3xl leading-[1.18] tracking-tight md:text-[2.5rem]'>
            {t('Every request takes the same path')}
          </h2>
          <p className='text-muted-foreground/80 mt-4 max-w-lg text-[15px] leading-relaxed'>
            {t(
              'From your client to the upstream model and back — and exactly where the time goes.'
            )}
          </p>
        </AnimateInView>

        <AnimateInView animation='scale-in'>
          <Tabs value={activeId} onValueChange={setActiveId}>
            <TabsList
              variant='line'
              className='border-border/50 mb-8 flex-wrap border-b pb-2'
            >
              {ENDPOINTS.map((endpoint) => (
                <TabsTrigger key={endpoint.id} value={endpoint.id}>
                  {endpoint.label}
                </TabsTrigger>
              ))}
            </TabsList>

            {ENDPOINTS.map((endpoint) => (
              <TabsContent key={endpoint.id} value={endpoint.id}>
                <FlowDiagram demo={endpoint} />
              </TabsContent>
            ))}
          </Tabs>
        </AnimateInView>
      </div>
    </section>
  )
}
