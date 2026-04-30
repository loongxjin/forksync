import { useRef, useEffect, useState, useCallback } from 'react'
import { useTranslation } from 'react-i18next'
import {
  Sheet,
  SheetContent,
  SheetHeader,
  SheetTitle,
  SheetClose
} from '@/components/ui/sheet'
import { Button } from '@/components/ui/button'
import type { AgentStreamEvent } from '@/types/engine'
import { Terminal, X } from 'lucide-react'

interface AgentTerminalDrawerProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  repoName: string
  events: AgentStreamEvent[]
  isLive: boolean
}

export function AgentTerminalDrawer({
  open,
  onOpenChange,
  repoName,
  events,
  isLive
}: AgentTerminalDrawerProps): JSX.Element {
  const { t } = useTranslation()
  const scrollRef = useRef<HTMLDivElement>(null)
  const [autoScroll, setAutoScroll] = useState(true)
  const [userScrolled, setUserScrolled] = useState(false)

  // Auto-scroll to bottom when new events arrive
  useEffect(() => {
    if (!autoScroll || !scrollRef.current) return
    scrollRef.current.scrollTop = scrollRef.current.scrollHeight
  }, [events, autoScroll])

  const handleScroll = useCallback(() => {
    const el = scrollRef.current
    if (!el) return
    const nearBottom = el.scrollHeight - el.scrollTop - el.clientHeight < 20
    if (nearBottom) {
      setAutoScroll(true)
      setUserScrolled(false)
    } else {
      setAutoScroll(false)
      setUserScrolled(true)
    }
  }, [])

  // Resume auto-scroll if user scrolls back to bottom
  useEffect(() => {
    const el = scrollRef.current
    if (!el) return
    el.addEventListener('scroll', handleScroll)
    return () => el.removeEventListener('scroll', handleScroll)
  }, [handleScroll])

  const renderEvent = (ev: AgentStreamEvent, index: number): React.ReactNode => {
    switch (ev.t) {
      case 'start':
        return (
          <div key={index} className="text-emerald-400 font-medium">
            <span className="mr-1">▶</span>
            {t('agentTerminal.started', { agent: ev.agent ?? 'agent' })}
            {ev.files && ev.files.length > 0 && (
              <span className="text-emerald-400/70 ml-2">
                ({ev.files.length} files)
              </span>
            )}
          </div>
        )
      case 'stdout':
        return (
          <div key={index} className="text-gray-100 whitespace-pre-wrap break-words">
            {ev.d}
          </div>
        )
      case 'stderr':
        return (
          <div key={index} className="text-amber-400 whitespace-pre-wrap break-words">
            {ev.d}
          </div>
        )
      case 'tool':
        return (
          <div key={index} className="text-blue-400 text-xs">
            <span className="mr-1">🔧</span>
            {ev.name} {ev.path && <span className="text-blue-400/70">{ev.path}</span>}
          </div>
        )
      case 'done':
        return (
          <div key={index} className={ev.success ? 'text-emerald-400 font-medium' : 'text-red-400 font-medium'}>
            <span className="mr-1">{ev.success ? '✓' : '✗'}</span>
            {ev.summary ?? (ev.success ? t('agentTerminal.completed') : t('agentTerminal.failed'))}
          </div>
        )
      case 'error':
        return (
          <div key={index} className="text-red-400 whitespace-pre-wrap break-words">
            <span className="mr-1">✗</span>
            {ev.d}
          </div>
        )
      default:
        return (
          <div key={index} className="text-gray-300 whitespace-pre-wrap break-words">
            {ev.d}
          </div>
        )
    }
  }

  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent side="right" className="w-[600px] max-w-full bg-black/95 border-l border-border/20 flex flex-col">
        <SheetHeader className="shrink-0 border-b border-white/10 px-4 py-3">
          <div className="flex items-center justify-between">
            <div className="flex items-center gap-2">
              <SheetTitle className="text-sm font-medium text-white flex items-center gap-2">
                <Terminal size={14} />
                {t('agentTerminal.title')}
              </SheetTitle>
              <span className="text-xs text-muted-foreground">— {repoName}</span>
              {isLive && (
                <span className="inline-flex items-center gap-1 text-xs text-emerald-400">
                  <span className="inline-block h-1.5 w-1.5 rounded-full bg-emerald-400 animate-pulse" />
                  {t('agentTerminal.live')}
                </span>
              )}
            </div>
            <SheetClose asChild>
              <Button variant="ghost" size="icon" className="h-7 w-7 text-muted-foreground hover:text-white">
                <X size={14} />
              </Button>
            </SheetClose>
          </div>
        </SheetHeader>

        <div
          ref={scrollRef}
          className="flex-1 overflow-y-auto p-4 font-mono text-xs leading-relaxed space-y-1"
          style={{ scrollbarWidth: 'thin', scrollbarColor: 'rgba(255,255,255,0.2) transparent' }}
        >
          {events.length === 0 ? (
            <div className="text-muted-foreground text-center py-8">
              {t('agentTerminal.waiting')}
              {isLive && <span className="animate-pulse ml-1">_</span>}
            </div>
          ) : (
            events.map((ev, i) => renderEvent(ev, i))
          )}
          {isLive && events.length > 0 && (
            <div className="text-emerald-400 animate-pulse">_</div>
          )}
        </div>

        {userScrolled && isLive && (
          <div className="shrink-0 px-4 py-2 border-t border-white/10">
            <Button
              variant="ghost"
              size="sm"
              className="w-full text-xs text-muted-foreground hover:text-white"
              onClick={() => {
                setAutoScroll(true)
                setUserScrolled(false)
                if (scrollRef.current) {
                  scrollRef.current.scrollTop = scrollRef.current.scrollHeight
                }
              }}
            >
              {t('agentTerminal.resumeScroll')}
            </Button>
          </div>
        )}
      </SheetContent>
    </Sheet>
  )
}
