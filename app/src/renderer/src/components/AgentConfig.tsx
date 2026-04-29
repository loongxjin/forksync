import { useState, useEffect, useRef } from 'react'
import { useAgents } from '@/contexts/AgentContext'
import { useSettings } from '@/contexts/SettingsContext'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { CheckCircle2, XCircle, Clock, Trash2 } from 'lucide-react'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Separator } from '@/components/ui/separator'
import { useTranslation } from 'react-i18next'

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

const conflictModeOptions = [
  { value: 'agent_resolve', labelKey: 'settings.agent.modes.agentResolve', descKey: 'settings.agent.modes.agentResolveDesc' },
  { value: 'manual', labelKey: 'settings.agent.modes.manual', descKey: 'settings.agent.modes.manualDesc' }
]

const resolveStrategyOptions = [
  { value: 'preserve_ours', labelKey: 'settings.agent.resolveStrategies.preserveOurs', descKey: 'settings.agent.resolveStrategies.preserveOursDesc' },
  { value: 'preserve_theirs', labelKey: 'settings.agent.resolveStrategies.preserveTheirs', descKey: 'settings.agent.resolveStrategies.preserveTheirsDesc' },
  { value: 'balanced', labelKey: 'settings.agent.resolveStrategies.balanced', descKey: 'settings.agent.resolveStrategies.balancedDesc' }
]

export function AgentConfig(): JSX.Element {
  const { agents, sessions, refreshAgents, refreshSessions, cleanup, resetSession } = useAgents()
  const { engineConfig, configLoading, updateConfig } = useSettings()
  const { t } = useTranslation()

  // Local state for debounced inputs
  const [timeout, setTimeout_] = useState('')
  const [sessionTTL, setSessionTTL] = useState('')
  const [savingTimeout, setSavingTimeout] = useState(false)
  const [savingTTL, setSavingTTL] = useState(false)
  const [cleaning, setCleaning] = useState(false)
  const [resettingId, setResettingId] = useState<string | null>(null)
  const [configSaving, setConfigSaving] = useState(false)
  const configSavingRef = useRef(false)
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
    setCleaning(true)
    try {
      await cleanup()
    } finally {
      setCleaning(false)
    }
  }

  const handleReset = async (repoId: string): Promise<void> => {
    if (!confirm(t('settings.agent.resetConfirm', { repoId }))) return
    setResettingId(repoId)
    try {
      await resetSession(repoId)
    } finally {
      setResettingId(null)
    }
  }

  const wrapConfigSave = async (fn: () => Promise<void>): Promise<void> => {
    if (configSavingRef.current) return
    configSavingRef.current = true
    setConfigSaving(true)
    try {
      await fn()
    } finally {
      configSavingRef.current = false
      setConfigSaving(false)
    }
  }

  const handleSetPreferred = async (name: string): Promise<void> => {
    await wrapConfigSave(async () => {
      await updateConfig('agent.preferred', name)
      refreshAgents()
    })
  }

  const handleConflictModeChange = async (mode: string): Promise<void> => {
    await wrapConfigSave(async () => {
      await updateConfig('agent.conflict_strategy', mode)
    })
  }

  const handleResolveStrategyChange = async (strategy: string): Promise<void> => {
    await wrapConfigSave(async () => {
      await updateConfig('agent.resolve_strategy', strategy)
    })
  }

  const handleAutoConfirm = async (val: boolean): Promise<void> => {
    await wrapConfigSave(async () => {
      await updateConfig('agent.confirm_before_commit', String(!val))
    })
  }

  const isLoading = configLoading || !engineConfig || configSaving
  const currentPreferred = engineConfig?.Agent?.Preferred || ''
  const currentConflictMode = engineConfig?.Agent?.ConflictStrategy || 'agent_resolve'
  const currentResolveStrategy = engineConfig?.Agent?.ResolveStrategy || 'preserve_ours'
  const isAgentResolveMode = currentConflictMode === 'agent_resolve'
  const autoConfirmEnabled = !(engineConfig?.Agent?.ConfirmBeforeCommit ?? true)

  return (
    <div className="space-y-4">
      {/* Detected Agents */}
      <div className="space-y-2">
        <Label className="text-sm font-medium">{t('settings.agent.detectedAgents')}</Label>
        <div className="space-y-2">
          {agents.map((agent) => (
            <div
              key={agent.name}
              className="flex items-center justify-between rounded-md border border-border bg-card p-3"
            >
              <div className="flex items-center gap-2">
                <span>{agent.installed ? <CheckCircle2 size={14} className="text-success" /> : <XCircle size={14} className="text-muted-foreground/50" />}</span>
                <span className="text-sm font-medium">{agent.name}</span>
                {agent.version && (
                  <span className="text-xs text-muted-foreground">v{agent.version}</span>
                )}
                {agent.name === currentPreferred && (
                  <Badge variant="info">{t('settings.agent.preferred')}</Badge>
                )}
              </div>
              <div className="flex items-center gap-2">
                <span className="text-xs text-muted-foreground">
                  {agent.installed ? agent.path : t('settings.agent.notInstalled')}
                </span>
                {agent.installed && agent.name !== currentPreferred && (
                  <Button
                    variant="outline"
                    size="sm"
                    onClick={() => handleSetPreferred(agent.name)}
                    disabled={isLoading}
                  >
                    {t('settings.agent.setPreferred')}
                  </Button>
                )}
              </div>
            </div>
          ))}
          {agents.length === 0 && (
            <p className="text-sm text-muted-foreground">{t('settings.agent.noAgentsDetected')}</p>
          )}
        </div>
      </div>

      <Separator />

      {/* Agent Configuration */}
      <div className="space-y-4">
        <Label className="text-sm font-medium">{t('settings.agent.config')}</Label>

        {/* Timeout */}
        <div className="space-y-1">
          <Label className="text-xs text-muted-foreground">{t('settings.agent.timeout')}</Label>
          <div className="flex items-center gap-2">
            <Input
              value={timeout}
              onChange={(e) => setTimeout_(e.target.value)}
              placeholder={t('settings.agent.timeoutPlaceholder')}
              className="max-w-[200px]"
              disabled={isLoading}
            />
            {savingTimeout && <span className="text-xs text-muted-foreground">{t('common.saving')}</span>}
          </div>
        </div>

        {/* Conflict Mode */}
        <div className="space-y-2">
          <Label className="text-xs text-muted-foreground">{t('settings.agent.conflictMode')}</Label>
          <div className="space-y-1">
            {conflictModeOptions.map((s) => (
              <label
                key={s.value}
                className={`flex cursor-pointer items-start gap-2 rounded-md border p-2 transition-colors ${
                  currentConflictMode === s.value
                    ? 'border-primary bg-primary/5'
                    : 'border-border hover:bg-accent/30'
                }`}
              >
                <input
                  type="radio"
                  name="conflict_mode"
                  value={s.value}
                  checked={currentConflictMode === s.value}
                  onChange={() => handleConflictModeChange(s.value)}
                  disabled={isLoading}
                  className="mt-0.5"
                />
                <div>
                  <div className="text-sm font-medium">{t(s.labelKey)}</div>
                  <div className="text-xs text-muted-foreground">{t(s.descKey)}</div>
                </div>
              </label>
            ))}
          </div>

          {/* Resolve Strategy (sub-option, only shown when agent_resolve is selected) */}
          {isAgentResolveMode && (
            <div className="ml-6 mt-2 space-y-1">
              <Label className="text-xs text-muted-foreground">{t('settings.agent.resolveStrategy')}</Label>
              {resolveStrategyOptions.map((s) => (
                <label
                  key={s.value}
                  className={`flex cursor-pointer items-start gap-2 rounded-md border p-2 transition-colors ${
                    currentResolveStrategy === s.value
                      ? 'border-primary bg-primary/5'
                      : 'border-border hover:bg-accent/30'
                  }`}
                >
                  <input
                    type="radio"
                    name="resolve_strategy"
                    value={s.value}
                    checked={currentResolveStrategy === s.value}
                    onChange={() => handleResolveStrategyChange(s.value)}
                    disabled={isLoading}
                    className="mt-0.5"
                  />
                  <div>
                    <div className="text-sm font-medium">{t(s.labelKey)}</div>
                    <div className="text-xs text-muted-foreground">{t(s.descKey)}</div>
                  </div>
                </label>
              ))}
            </div>
          )}
        </div>

        {/* Auto Confirm */}
        <Toggle
          label={t('settings.agent.autoConfirm')}
          checked={autoConfirmEnabled}
          onChange={handleAutoConfirm}
          disabled={isLoading}
        />
        <p className="text-xs text-muted-foreground -mt-3">
          {t('settings.agent.autoConfirmDesc')}
        </p>

        {/* Session TTL */}
        <div className="space-y-1">
          <Label className="text-xs text-muted-foreground">{t('settings.agent.sessionTTL')}</Label>
          <div className="flex items-center gap-2">
            <Input
              value={sessionTTL}
              onChange={(e) => setSessionTTL(e.target.value)}
              placeholder={t('settings.agent.sessionTTLPlaceholder')}
              className="max-w-[200px]"
              disabled={isLoading}
            />
            {savingTTL && <span className="text-xs text-muted-foreground">{t('common.saving')}</span>}
          </div>
        </div>
      </div>

      <Separator />

      {/* Sessions */}
      <div className="space-y-2">
        <div className="flex items-center justify-between">
          <Label className="text-sm font-medium">
            {t('settings.agent.sessions', { count: sessions.length })}
          </Label>
          <Button variant="outline" size="sm" onClick={handleCleanup} disabled={cleaning}>
            {cleaning ? t('common.processing') : t('settings.agent.cleanupExpired')}
          </Button>
        </div>
        {sessions.length === 0 ? (
          <p className="text-sm text-muted-foreground">{t('settings.agent.noSessions')}</p>
        ) : (
          <div className="space-y-1">
            {sessions.map((s) => (
              <div
                key={s.id}
                className="flex items-center justify-between rounded-md border border-border bg-card p-2 text-xs"
              >
                <div className="flex items-center gap-2">
                  <span>
                    {s.status === 'active' ? <CheckCircle2 size={12} className="text-success" /> : s.status === 'expired' ? <Clock size={12} className="text-warning" /> : <XCircle size={12} className="text-muted-foreground/50" />}
                  </span>
                  <span className="font-medium">{s.agentName}</span>
                  <span className="text-muted-foreground">{s.repoId}</span>
                </div>
                <div className="flex items-center gap-2">
                  <span className="text-muted-foreground">{s.status}</span>
                  <Button
                    variant="ghost"
                    size="sm"
                    className="h-6 w-6 p-0 text-muted-foreground hover:text-destructive"
                    onClick={() => handleReset(s.repoId)}
                    disabled={resettingId === s.repoId}
                    title={t('settings.agent.resetSession')}
                  >
                    <Trash2 size={12} />
                  </Button>
                </div>
              </div>
            ))}
          </div>
        )}
      </div>
    </div>
  )
}
