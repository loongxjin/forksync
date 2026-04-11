import { useState, useEffect, useRef } from 'react'
import { useAgents } from '@/contexts/AgentContext'
import { useSettings } from '@/contexts/SettingsContext'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Separator } from '@/components/ui/separator'

/** Toggle reused from other settings panels */
function Toggle({
  checked,
  onChange,
  disabled = false,
  label
}: {
  checked: boolean
  onChange: (val: boolean) => void
  disabled?: boolean
  label: string
}): JSX.Element {
  return (
    <div className="flex items-center justify-between">
      <Label className="cursor-pointer">{label}</Label>
      <button
        role="switch"
        aria-checked={checked}
        disabled={disabled}
        onClick={() => onChange(!checked)}
        className={`relative inline-flex h-5 w-9 shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:cursor-not-allowed disabled:opacity-50 ${
          checked ? 'bg-primary' : 'bg-input'
        }`}
      >
        <span
          className={`pointer-events-none block h-4 w-4 rounded-full bg-background shadow-lg ring-0 transition-transform ${
            checked ? 'translate-x-4' : 'translate-x-0'
          }`}
        />
      </button>
    </div>
  )
}

const conflictStrategies = [
  { value: 'preserve_ours', label: 'Preserve Ours', desc: 'Keep local changes, accept upstream non-conflict changes' },
  { value: 'preserve_theirs', label: 'Preserve Theirs', desc: 'Accept upstream changes, keep local non-conflict changes' },
  { value: 'agent_resolve', label: 'Agent Resolve', desc: 'Let AI agent resolve conflicts automatically' },
  { value: 'manual', label: 'Manual', desc: 'Always resolve conflicts manually' }
]

export function AgentConfig(): JSX.Element {
  const { agents, sessions, refreshAgents, refreshSessions, cleanup } = useAgents()
  const { engineConfig, configLoading, updateConfig } = useSettings()

  // Local state for debounced inputs
  const [timeout, setTimeout_] = useState('')
  const [sessionTTL, setSessionTTL] = useState('')
  const [savingTimeout, setSavingTimeout] = useState(false)
  const [savingTTL, setSavingTTL] = useState(false)
  const isEditingRef = useRef({ timeout: false, sessionTTL: false })
  const prevConfigRef = useRef({ timeout: '', sessionTTL: '' })

  // Sync from engine config (only when not editing)
  useEffect(() => {
    if (engineConfig?.Agent) {
      const cfgTimeout = engineConfig.Agent.Timeout || ''
      const cfgTTL = engineConfig.Agent.SessionTTL || ''
      if (!isEditingRef.current.timeout && cfgTimeout !== prevConfigRef.current.timeout) {
        setTimeout_(cfgTimeout)
        prevConfigRef.current.timeout = cfgTimeout
      }
      if (!isEditingRef.current.sessionTTL && cfgTTL !== prevConfigRef.current.sessionTTL) {
        setSessionTTL(cfgTTL)
        prevConfigRef.current.sessionTTL = cfgTTL
      }
    }
  }, [engineConfig])

  // Debounced save: timeout
  useEffect(() => {
    if (!timeout || !engineConfig) return
    if (timeout === engineConfig.Agent?.Timeout) return
    isEditingRef.current.timeout = true
    const timer = setTimeout(async () => {
      setSavingTimeout(true)
      await updateConfig('agent.timeout', timeout)
      setSavingTimeout(false)
      isEditingRef.current.timeout = false
      prevConfigRef.current.timeout = timeout
    }, 1500)
    return () => clearTimeout(timer)
  }, [timeout, engineConfig, updateConfig])

  // Debounced save: session TTL
  useEffect(() => {
    if (!sessionTTL || !engineConfig) return
    if (sessionTTL === engineConfig.Agent?.SessionTTL) return
    isEditingRef.current.sessionTTL = true
    const timer = setTimeout(async () => {
      setSavingTTL(true)
      await updateConfig('agent.session_ttl', sessionTTL)
      setSavingTTL(false)
      isEditingRef.current.sessionTTL = false
      prevConfigRef.current.sessionTTL = sessionTTL
    }, 1500)
    return () => clearTimeout(timer)
  }, [sessionTTL, engineConfig, updateConfig])

  useEffect(() => {
    refreshAgents()
    refreshSessions()
  }, [refreshAgents, refreshSessions])

  const handleCleanup = async (): Promise<void> => {
    await cleanup()
  }

  const handleSetPreferred = async (name: string): Promise<void> => {
    await updateConfig('agent.preferred', name)
    // Refresh agents to reflect new preferred
    refreshAgents()
  }

  const handleStrategyChange = async (strategy: string): Promise<void> => {
    await updateConfig('agent.conflict_strategy', strategy)
  }

  const handleAutoConfirm = async (val: boolean): Promise<void> => {
    await updateConfig('agent.confirm_before_commit', String(!val))
  }

  const isLoading = configLoading || !engineConfig
  const currentPreferred = engineConfig?.Agent?.Preferred || ''
  const currentStrategy = engineConfig?.Agent?.ConflictStrategy || 'preserve_ours'
  const autoConfirmEnabled = !(engineConfig?.Agent?.ConfirmBeforeCommit ?? true)

  return (
    <div className="space-y-4">
      {/* Detected Agents */}
      <div className="space-y-2">
        <Label className="text-sm font-medium">Detected Agents</Label>
        <div className="space-y-2">
          {agents.map((agent) => (
            <div
              key={agent.name}
              className="flex items-center justify-between rounded-md border border-border bg-card p-3"
            >
              <div className="flex items-center gap-2">
                <span>{agent.installed ? '✅' : '❌'}</span>
                <span className="text-sm font-medium">{agent.name}</span>
                {agent.version && (
                  <span className="text-xs text-muted-foreground">v{agent.version}</span>
                )}
                {agent.name === currentPreferred && (
                  <Badge variant="info">preferred</Badge>
                )}
              </div>
              <div className="flex items-center gap-2">
                <span className="text-xs text-muted-foreground">
                  {agent.installed ? agent.path : 'not installed'}
                </span>
                {agent.installed && agent.name !== currentPreferred && (
                  <Button
                    variant="outline"
                    size="sm"
                    onClick={() => handleSetPreferred(agent.name)}
                    disabled={isLoading}
                  >
                    Set Preferred
                  </Button>
                )}
              </div>
            </div>
          ))}
          {agents.length === 0 && (
            <p className="text-sm text-muted-foreground">No agent CLIs detected.</p>
          )}
        </div>
      </div>

      <Separator />

      {/* Agent Configuration */}
      <div className="space-y-4">
        <Label className="text-sm font-medium">Agent Configuration</Label>

        {/* Timeout */}
        <div className="space-y-1">
          <Label className="text-xs text-muted-foreground">Agent Timeout</Label>
          <div className="flex items-center gap-2">
            <Input
              value={timeout}
              onChange={(e) => setTimeout_(e.target.value)}
              placeholder="e.g. 10m"
              className="max-w-[200px]"
              disabled={isLoading}
            />
            {savingTimeout && <span className="text-xs text-muted-foreground">Saving...</span>}
          </div>
        </div>

        {/* Conflict Strategy */}
        <div className="space-y-2">
          <Label className="text-xs text-muted-foreground">Conflict Strategy</Label>
          <div className="space-y-1">
            {conflictStrategies.map((s) => (
              <label
                key={s.value}
                className={`flex cursor-pointer items-start gap-2 rounded-md border p-2 transition-colors ${
                  currentStrategy === s.value
                    ? 'border-primary bg-primary/5'
                    : 'border-border hover:bg-accent/30'
                }`}
              >
                <input
                  type="radio"
                  name="conflict_strategy"
                  value={s.value}
                  checked={currentStrategy === s.value}
                  onChange={() => handleStrategyChange(s.value)}
                  disabled={isLoading}
                  className="mt-0.5"
                />
                <div>
                  <div className="text-sm font-medium">{s.label}</div>
                  <div className="text-xs text-muted-foreground">{s.desc}</div>
                </div>
              </label>
            ))}
          </div>
        </div>

        {/* Auto Confirm */}
        <Toggle
          label="Auto-confirm Agent Results"
          checked={autoConfirmEnabled}
          onChange={handleAutoConfirm}
          disabled={isLoading}
        />
        <p className="text-xs text-muted-foreground -mt-3">
          Skip manual confirmation when agent resolves conflicts successfully
        </p>

        {/* Session TTL */}
        <div className="space-y-1">
          <Label className="text-xs text-muted-foreground">Session TTL</Label>
          <div className="flex items-center gap-2">
            <Input
              value={sessionTTL}
              onChange={(e) => setSessionTTL(e.target.value)}
              placeholder="e.g. 24h"
              className="max-w-[200px]"
              disabled={isLoading}
            />
            {savingTTL && <span className="text-xs text-muted-foreground">Saving...</span>}
          </div>
        </div>
      </div>

      <Separator />

      {/* Sessions */}
      <div className="space-y-2">
        <div className="flex items-center justify-between">
          <Label className="text-sm font-medium">
            Sessions ({sessions.length})
          </Label>
          <Button variant="outline" size="sm" onClick={handleCleanup}>
            🧹 Cleanup Expired
          </Button>
        </div>
        {sessions.length === 0 ? (
          <p className="text-sm text-muted-foreground">No active sessions.</p>
        ) : (
          <div className="space-y-1">
            {sessions.map((s) => (
              <div
                key={s.id}
                className="flex items-center justify-between rounded-md border border-border bg-card p-2 text-xs"
              >
                <div className="flex items-center gap-2">
                  <span>
                    {s.status === 'active' ? '🟢' : s.status === 'expired' ? '⏰' : '❌'}
                  </span>
                  <span className="font-medium">{s.agentName}</span>
                  <span className="text-muted-foreground">{s.repoId}</span>
                </div>
                <span className="text-muted-foreground">{s.status}</span>
              </div>
            ))}
          </div>
        )}
      </div>
    </div>
  )
}
