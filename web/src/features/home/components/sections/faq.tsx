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
import { useEffect, useMemo } from 'react'
import { useTranslation } from 'react-i18next'

import {
  Accordion,
  AccordionContent,
  AccordionItem,
  AccordionTrigger,
} from '@/components/ui/accordion'

import { Reveal } from '../motion'

/** Question/answer pairs. Order here drives both the visible accordion and the
 *  FAQPage structured data, so the two never drift apart. */
const FAQ_ITEMS = [
  {
    q: 'What is an AI API gateway?',
    a: 'An AI API gateway is a single endpoint that routes your requests to many upstream model providers. Instead of integrating OpenAI, Claude, Gemini, and others separately, you call one OpenAI-compatible API and the gateway handles authentication, protocol translation, load balancing, and billing.',
  },
  {
    q: 'Which AI models and providers are supported?',
    a: 'It aggregates 40+ upstream providers, including OpenAI, Anthropic Claude, Google Gemini, Azure OpenAI, AWS Bedrock, DeepSeek, and Qwen, behind one unified protocol. New channels can be added as providers ship.',
  },
  {
    q: 'Is it OpenAI API compatible?',
    a: 'Yes. Requests and responses follow the OpenAI-compatible format, so existing SDKs and tools work by changing only the base URL and API key. Native Claude and Gemini endpoints are also exposed for clients that need them.',
  },
  {
    q: 'How is usage billed?',
    a: 'Billing is pay-as-you-go and transparent: every request records prompt and completion tokens, latency, and cost. You can review usage in real time, set quotas and rate limits per user or token, and see the cost of each request before you commit.',
  },
  {
    q: 'Can I self-host the gateway?',
    a: 'Yes. The gateway is self-hostable and open source, with SQLite, MySQL, or PostgreSQL for the primary database and optional Redis and ClickHouse for caching and logs. You keep control of your keys, data, and deployment.',
  },
  {
    q: 'How does it handle reliability and failover?',
    a: 'Requests are routed to the healthiest upstream based on live channel status. Unhealthy channels are taken out of rotation automatically, and load balancing spreads traffic across the providers you configure so a single outage does not take you down.',
  },
]

/**
 * FAQ section + FAQPage structured data. The visible accordion is the source of
 * truth; the JSON-LD is generated from the same translated strings so the markup
 * a crawler reads always matches what a user sees. The script is replaced on
 * language change and removed on unmount.
 */
export function FAQ() {
  const { t, i18n } = useTranslation()

  // `items` must stay referentially stable across renders, otherwise the JSON-LD
  // effect below would tear down and re-append the script on every render. It
  // changes only when the language (and therefore the translation) changes.
  const items = useMemo(
    () =>
      FAQ_ITEMS.map((item) => ({
        question: t(item.q),
        answer: t(item.a),
      })),
    // eslint-disable-next-line react-hooks/exhaustive-deps -- re-derive on language change only
    [i18n.language]
  )

  useEffect(() => {
    const script = document.createElement('script')
    script.type = 'application/ld+json'
    script.id = 'landing-faq-jsonld'
    script.textContent = JSON.stringify({
      '@context': 'https://schema.org',
      '@type': 'FAQPage',
      mainEntity: items.map((item) => ({
        '@type': 'Question',
        name: item.question,
        acceptedAnswer: { '@type': 'Answer', text: item.answer },
      })),
    })
    document.head.appendChild(script)
    return () => {
      script.remove()
    }
    // Re-emit when the rendered answers change (language switch).
  }, [items])

  return (
    <section className='relative z-10 px-6 py-28 md:py-36'>
      <div className='mx-auto max-w-3xl'>
        <Reveal className='mb-14 text-center'>
          <p className='text-muted-foreground/70 mb-4 text-[11px] font-medium tracking-[0.28em] uppercase'>
            {t('FAQ')}
          </p>
          <h2 className='font-serif text-3xl leading-[1.18] tracking-tight md:text-[2.5rem]'>
            {t('Frequently asked questions')}
          </h2>
        </Reveal>

        <Reveal distance={16}>
          <Accordion className='border-border/40 w-full'>
            {items.map((item) => (
              <AccordionItem key={item.question} value={item.question}>
                <AccordionTrigger className='text-left text-[15px] font-medium'>
                  {item.question}
                </AccordionTrigger>
                <AccordionContent className='text-muted-foreground/85 text-sm leading-relaxed'>
                  {item.answer}
                </AccordionContent>
              </AccordionItem>
            ))}
          </Accordion>
        </Reveal>
      </div>
    </section>
  )
}
