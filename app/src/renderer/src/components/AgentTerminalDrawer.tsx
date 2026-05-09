import { useRef, useEffect, useState, useCallback, useMemo } from 'react'
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
import { Terminal, X, Wrench, AlertTriangle, CheckCircle2, XCircle, Play } from 'lucide-react'
import { cn } from '@/lib/utils'

interface AgentTerminalDrawerProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  repoName: string
  events: AgentStreamEvent[]
  isLive: boolean
}

/** Format relative time from start timestamp */
function formatRelative(ts: string, startTs: string): string {
  const start = new Date(startTs).getTime()
  const current = new Date(ts).getTime()
  if (isNaN(start) || isNaN(current)) return ''
  const diffMs = current - start
  if (diffMs < 0) return '+0s'
  const diffSec = Math.round(diffMs / 1000)
  if (diffSec < 60) return `+${diffSec}s`
  const min = Math.floor(diffSec / 60)
  const sec = diffSec % 60
  return `+${min}m${sec}s`
}

/** Format duration between two timestamps */
function formatDuration(startTs: string, endTs: string): string {
  const start = new Date(startTs).getTime()
  const end = new Date(endTs).getTime()
  if (isNaN(start) || isNaN(end)) return ''
  const diffSec = Math.round((end - start) / 1000)
  if (diffSec < 60) return `${diffSec}s`
  const min = Math.floor(diffSec / 60)
  const sec = diffSec % 60
  return `${min}m${sec}s`
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
  const [drawerWidth, setDrawerWidth] = useState(600)
  const isDragging = useRef(false)
  const prevEventsLenRef = useRef(0)

  // Debug: log when events change
  useEffect(() => {
    if (events.length !== prevEventsLenRef.current) {
      const typeDist: Record<string, number> = {}
      for (const ev of events) { typeDist[ev.t] = (typeDist[ev.t] || 0) + 1 }
      console.log('[AgentTerminal] events changed for', repoName, 'count:', events.length, '(was:', prevEventsLenRef.current, '), types:', JSON.stringify(typeDist), ', isLive:', isLive)
      prevEventsLenRef.current = events.length
    }
  }, [events, repoName, isLive])

  // Compute start timestamp for relative times
  const startTs = useMemo(() => {
    const startEvent = events.find((e) => e.t === 'start')
    return startEvent?.ts ?? ''
  }, [events])

  // Compute stats
  const toolCount = useMemo(
    () => events.filter((e) => e.t === 'tool').length,
    [events]
  )
  const doneEvent = useMemo(
    () => events.find((e) => e.t === 'done'),
    [events]
  )

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

  // Drag-to-resize handlers
  const handleMouseDown = useCallback((e: React.MouseEvent) => {
    e.preventDefault()
    isDragging.current = true
    const startX = e.clientX
    const startWidth = drawerWidth

    const onMouseMove = (moveEvent: MouseEvent) => {
      if (!isDragging.current) return
      const delta = startX - moveEvent.clientX
      const newWidth = Math.min(900, Math.max(400, startWidth + delta))
      setDrawerWidth(newWidth)
    }

    const onMouseUp = () => {
      isDragging.current = false
      document.removeEventListener('mousemove', onMouseMove)
      document.removeEventListener('mouseup', onMouseUp)
      document.body.style.cursor = ''
      document.body.style.userSelect = ''
    }

    document.body.style.cursor = 'col-resize'
    document.body.style.userSelect = 'none'
    document.addEventListener('mousemove', onMouseMove)
    document.addEventListener('mouseup', onMouseUp)
  }, [drawerWidth])

  const renderEvent = (ev: AgentStreamEvent, index: number): React.ReactNode => {
    const relTime = startTs ? formatRelative(ev.ts, startTs) : ''

    switch (ev.t) {
      case 'start':
        return (
          <div
            key={index}
            className="rounded-md border-l-[3px] border-l-emerald-400 bg-emerald-400/5 px-3 py-2"
          >
            <div className="flex items-center justify-between">
              <div className="flex items-center gap-2">
                <Play size={12} className="text-emerald-400 shrink-0" />
                <span className="text-emerald-400 font-medium text-xs">
                  {t('agentTerminal.started', { agent: ev.agent ?? 'agent' })}
                </span>
              </div>
              {relTime && (
                <span className="text-[10px] text-emerald-400/50 tabular-nums font-mono">{relTime}</span>
              )}
            </div>
            {ev.files && ev.files.length > 0 && (
              <span className="text-[11px] text-emerald-400/60 ml-5">
                {t('agentTerminal.filesCount', { count: ev.files.length })}
              </span>
            )}
          </div>
        )
      case 'stdout':
        return (
          <div key={index} className="text-gray-400 whitespace-pre-wrap break-words text-xs leading-relaxed">
            {relTime && <span className="text-[10px] text-gray-600 tabular-nums font-mono mr-2 select-none">{relTime}</span>}
            {ev.d}
          </div>
        )
      case 'stderr':
        return (
          <div
            key={index}
            className="border-l-[3px] border-l-amber-400/60 pl-2 text-amber-400/90 whitespace-pre-wrap break-words text-xs leading-relaxed"
          >
            {relTime && <span className="text-[10px] text-amber-400/40 tabular-nums font-mono mr-1 select-none">{relTime}</span>}
            {ev.d}
          </div>
        )
      case 'tool':
        return (
          <div key={index} className="flex items-start gap-1.5 pl-2 py-0.5">
            {relTime && (
              <span className="text-[10px] text-gray-600 tabular-nums font-mono shrink-0 pt-[1px] select-none">{relTime}</span>
            )}
            <Wrench size={11} className="text-blue-400 shrink-0 mt-[2px]" />
            <span className="text-blue-300 font-medium text-xs">{ev.name}</span>
            {ev.path && (
              <span className="text-blue-400/50 font-mono text-[11px] truncate">{ev.path}</span>
            )}
          </div>
        )
      case 'done': {
        const duration = startTs ? formatDuration(startTs, ev.ts) : ''
        const isSuccess = ev.success !== false
        return (
          <div
            key={index}
            className={cn(
              'rounded-md border-l-[3px] px-3 py-2',
              isSuccess ? 'border-l-emerald-400 bg-emerald-400/5' : 'border-l-red-400 bg-red-400/5'
            )}
          >
            <div className="flex items-center justify-between">
              <div className="flex items-center gap-2">
                {isSuccess ? (
                  <CheckCircle2 size={12} className="text-emerald-400 shrink-0" />
                ) : (
                  <XCircle size={12} className="text-red-400 shrink-0" />
                )}
                <span className={cn('font-medium text-xs', isSuccess ? 'text-emerald-400' : 'text-red-400')}>
                  {duration
                    ? t(`agentTerminal.${isSuccess ? 'completedIn' : 'failedIn'}`, { duration })
                    : (isSuccess ? t('agentTerminal.completed') : t('agentTerminal.failed'))}
                </span>
              </div>
              {relTime && (
                <span className={cn('text-[10px] tabular-nums font-mono', isSuccess ? 'text-emerald-400/50' : 'text-red-400/50')}>
                  {relTime}
                </span>
              )}
            </div>
            {ev.summary && (
              <p className={cn('text-xs mt-1 ml-5', isSuccess ? 'text-emerald-400/70' : 'text-red-400/70')}>
                {ev.summary}
              </p>
            )}
          </div>
        )
      }
      case 'error':
        return (
          <div
            key={index}
            className="rounded-md border-l-[3px] border-l-red-400 bg-red-400/5 px-3 py-2"
          >
            <div className="flex items-center justify-between">
              <div className="flex items-center gap-2">
                <AlertTriangle size={12} className="text-red-400 shrink-0" />
                <span className="text-red-400 text-xs font-medium whitespace-pre-wrap break-words">{ev.d}</span>
              </div>
              {relTime && (
                <span className="text-[10px] text-red-400/50 tabular-nums font-mono shrink-0">{relTime}</span>
              )}
            </div>
          </div>
        )
      default:
        return (
          <div key={index} className="text-gray-300 whitespace-pre-wrap break-words text-xs">
            {relTime && <span className="text-[10px] text-gray-600 tabular-nums font-mono mr-2 select-none">{relTime}</span>}
            {ev.d}
          </div>
        )
    }
  }

  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent
        side="right"
        className="flex flex-col p-0"
        style={{ width: `${drawerWidth}px`, maxWidth: '100%' }}
      >
        {/* Dark terminal bg applied as inner wrapper to keep resize handle on card bg */}
        <div className="flex flex-col h-full bg-black/95 border-l border-border/20">
          {/* Resize handle */}
          <div
            className="absolute left-0 top-0 bottom-0 w-1.5 cursor-col-resize z-10 group"
            onMouseDown={handleMouseDown}
          >
            <div className="absolute left-0 top-1/2 -translate-y-1/2 w-1 h-8 rounded-r bg-white/0 group-hover:bg-white/20 transition-colors duration-150" />
          </div>

          {/* Header */}
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

          {/* Event log */}
          <div
            ref={scrollRef}
            className="flex-1 overflow-y-auto p-4 font-mono text-xs leading-relaxed space-y-1.5"
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

          {/* Status bar */}
          <div className="shrink-0 px-4 py-2 border-t border-white/10 flex items-center justify-between text-[11px] text-gray-500">
            <span>
              {t('agentTerminal.statusBar', { events: events.length, tools: toolCount })}
            </span>
            <span>
              {doneEvent
                ? doneEvent.success !== false
                  ? t('agentTerminal.completed')
                  : t('agentTerminal.failed')
                : isLive
                  ? <span className="text-emerald-400 flex items-center gap-1">
                      <span className="inline-block h-1.5 w-1.5 rounded-full bg-emerald-400 animate-pulse" />
                      {t('agentTerminal.live')}
                    </span>
                  : ''
              }
            </span>
          </div>

          {/* Resume scroll button */}
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
        </div>
      </SheetContent>
    </Sheet>
  )
}


