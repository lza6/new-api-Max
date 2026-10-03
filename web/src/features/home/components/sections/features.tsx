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
import {
  Zap,
  Shield,
  Globe,
  Code,
  Gauge,
  DollarSign,
  Users,
  HeartHandshake,
  FileArchive,
} from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { AnimateInView } from '@/components/animate-in-view'

import { SpotlightCard, TiltCard } from '../motion'

interface FeaturesProps {
  className?: string
}

export function Features(_props: FeaturesProps) {
  const { t } = useTranslation()

  const features = [
    {
      id: 'fast',
      num: '01',
      title: t('Lightning Fast'),
      desc: t(
        'Optimized network architecture ensures millisecond response times'
      ),
      span: 'md:col-span-2',
      icon: <Zap className='size-4 text-blue-400' />,
      visual: (
        <div className='mt-4 grid grid-cols-3 gap-2'>
          {['OpenAI', 'Claude', 'Gemini', 'DeepSeek', 'Qwen', 'Llama'].map(
            (name) => (
              <div
                key={name}
                className='border-border/30 bg-muted/20 text-muted-foreground flex items-center justify-center rounded-lg border px-3 py-2 text-xs transition-colors duration-300 hover:border-blue-500/30 hover:bg-blue-500/5'
              >
                {name}
              </div>
            )
          )}
        </div>
      ),
    },
    {
      id: 'secure',
      num: '02',
      title: t('Secure & Reliable'),
      desc: t(
        'Enterprise-grade security with comprehensive permission management'
      ),
      span: 'md:col-span-1',
      icon: <Shield className='size-4 text-emerald-400' />,
      visual: (
        <div className='mt-4 flex items-center justify-center'>
          <div className='relative'>
            <div className='flex size-16 items-center justify-center rounded-2xl border border-emerald-500/20 bg-emerald-500/5'>
              <Shield
                className='size-7 text-emerald-500/70'
                strokeWidth={1.5}
              />
            </div>
            <div className='absolute -top-1 -right-1 flex size-4 items-center justify-center rounded-full bg-emerald-500'>
              <svg
                className='size-2.5 text-white'
                fill='none'
                viewBox='0 0 24 24'
                stroke='currentColor'
                strokeWidth={3}
              >
                <path
                  strokeLinecap='round'
                  strokeLinejoin='round'
                  d='m4.5 12.75 6 6 9-13.5'
                />
              </svg>
            </div>
          </div>
        </div>
      ),
    },
    {
      id: 'global',
      num: '03',
      title: t('Global Coverage'),
      desc: t('Multi-region deployment for stable global access'),
      span: 'md:col-span-1',
      icon: <Globe className='size-4 text-violet-400' />,
      visual: (
        <div className='mt-4 space-y-2'>
          {[t('Load Balancing'), t('Rate Limiting'), t('Cost Tracking')].map(
            (step, i) => (
              <div key={step} className='flex items-center gap-2'>
                <div
                  className={`flex size-6 items-center justify-center rounded-full text-[10px] font-bold ${
                    i === 1
                      ? 'border border-blue-500/30 bg-blue-500/20 text-blue-500'
                      : 'border-border/40 bg-muted text-muted-foreground border'
                  }`}
                >
                  {i + 1}
                </div>
                <div className='bg-border/40 h-px flex-1' />
                <span className='text-muted-foreground text-xs'>{step}</span>
              </div>
            )
          )}
        </div>
      ),
    },
    {
      id: 'developer',
      num: '04',
      title: t('Developer Friendly'),
      desc: t('Compatible API routes for common AI application workflows'),
      span: 'md:col-span-2',
      icon: <Code className='size-4 text-amber-400' />,
      visual: (
        <div className='mt-4 flex items-center gap-3'>
          <div className='flex -space-x-2'>
            {['API', 'SDK', 'CLI', 'Docs'].map((n) => (
              <div
                key={n}
                className='border-background from-muted to-muted/60 text-muted-foreground flex size-8 items-center justify-center rounded-full border-2 bg-gradient-to-br text-[9px] font-bold'
              >
                {n}
              </div>
            ))}
          </div>
          <div className='text-muted-foreground flex items-center gap-1.5 text-xs'>
            <Code className='size-3.5 text-blue-500' />
            {t('Multi-protocol Compatible')}
          </div>
        </div>
      ),
    },
    {
      id: 'compression',
      num: '05',
      title: t('Request Body Compression'),
      desc: t(
        'Large prompts are gzip-compressed before leaving the gateway, cutting upload bandwidth and first-token latency. On by default, with automatic fallback for upstreams that reject it.'
      ),
      span: 'md:col-span-3',
      icon: <FileArchive className='size-4 text-cyan-400' />,
      visual: (
        <div className='mt-4 flex flex-col gap-2'>
          {[
            { before: '2.1 MB', after: '14 KB', ratio: '99.4%', noteKey: 'code & text' },
            { before: '700 KB', after: '184 KB', ratio: '73.7%', noteKey: 'typical prompt' },
          ].map((row) => (
            <div key={row.before} className='flex items-center gap-3 text-xs'>
              <span className='text-muted-foreground w-16 shrink-0 tabular-nums'>
                {row.before}
              </span>
              <div className='bg-muted/40 relative h-1.5 flex-1 overflow-hidden rounded-full'>
                <div
                  className='absolute inset-y-0 left-0 rounded-full bg-gradient-to-r from-cyan-500 to-blue-500'
                  style={{ width: `${100 - Number.parseFloat(row.ratio)}%` }}
                />
              </div>
              <span className='w-16 shrink-0 text-right font-mono tabular-nums text-cyan-600 dark:text-cyan-400'>
                {row.after}
              </span>
              <span className='text-muted-foreground/70 hidden w-24 shrink-0 text-right text-[10px] sm:block'>
                {t(row.noteKey)}
              </span>
            </div>
          ))}
        </div>
      ),
    },
  ]

  const additionalFeatures = [
    {
      icon: <Gauge className='size-5' strokeWidth={1.5} />,
      title: t('High Performance'),
      desc: t('Support for high concurrency with automatic load balancing'),
    },
    {
      icon: <DollarSign className='size-5' strokeWidth={1.5} />,
      title: t('Transparent Billing'),
      desc: t('Pay-as-you-go with real-time usage monitoring'),
    },
    {
      icon: <Users className='size-5' strokeWidth={1.5} />,
      title: t('Team Collaboration'),
      desc: t('Multi-user management with flexible permission allocation'),
    },
    {
      icon: <HeartHandshake className='size-5' strokeWidth={1.5} />,
      title: t('Open Source'),
      desc: t('Community driven, self-hosted, and extensible'),
    },
  ]

  return (
    <section className='relative z-10 px-6 py-28 md:py-36'>
      <div className='mx-auto max-w-6xl'>
        <AnimateInView className='mb-16 max-w-lg'>
          <p className='text-muted-foreground/70 mb-4 text-[11px] font-medium tracking-[0.28em] uppercase'>
            {t('Core Features')}
          </p>
          <h2 className='font-serif text-3xl leading-[1.18] tracking-tight md:text-[2.5rem]'>
            {t('Built for developers,')}
            <br />
            {t('designed for scale')}
          </h2>
        </AnimateInView>

        {/* Bento grid */}
        <div className='border-border/40 bg-border/40 grid gap-px overflow-hidden rounded-xl border md:grid-cols-3'>
          {features.map((f, i) => (
            <AnimateInView
              key={f.id}
              delay={i * 100}
              animation='scale-in'
              className={`group ${f.span}`}
            >
              {/* Cursor-following glow is clipped to the cell, so the shared
                  hairline grid stays intact while each panel lights up on hover. */}
              <SpotlightCard className='bg-background hover:bg-muted/20 h-full p-7 transition-colors duration-300 md:p-8'>
                <div className='mb-3 flex items-center gap-3'>
                  <span className='border-border/40 bg-muted text-muted-foreground flex size-7 items-center justify-center rounded-md border text-[10px] font-semibold tabular-nums'>
                    {f.num}
                  </span>
                  <h3 className='text-sm font-semibold'>{f.title}</h3>
                </div>
                <p className='text-muted-foreground text-sm leading-relaxed'>
                  {f.desc}
                </p>
                {f.visual}
              </SpotlightCard>
            </AnimateInView>
          ))}
        </div>

        {/* Additional features row */}
        <div className='mt-12 grid grid-cols-2 gap-8 md:grid-cols-4 md:gap-12'>
          {additionalFeatures.map((f, i) => (
            <AnimateInView key={f.title} delay={i * 100} animation='fade-up'>
              <TiltCard className='flex flex-col items-center text-center'>
                <div className='text-muted-foreground border-border/50 bg-muted/30 group-hover:text-foreground mb-3 flex size-12 items-center justify-center rounded-xl border transition-colors'>
                  {f.icon}
                </div>
                <h3 className='mb-1.5 text-sm font-semibold'>{f.title}</h3>
                <p className='text-muted-foreground max-w-[200px] text-xs leading-relaxed'>
                  {f.desc}
                </p>
              </TiltCard>
            </AnimateInView>
          ))}
        </div>
      </div>
    </section>
  )
}
