import { useState, useEffect, useRef, useCallback } from 'react'

type UpdateConfigFn = (key: string, value: string) => Promise<void>

/**
 * Hook for debounced config value saving.
 *
 * Manages local state for a config input, syncs from engine config,
 * and debounces saves (1500ms) to avoid excessive writes.
 *
 * @param configKey   - The engine config key (e.g. 'sync.default_interval')
 * @param configValue - The current value from engine config (used for sync comparison)
 * @param updateConfig - The updateConfig function from SettingsContext
 * @returns [localValue, setLocalValue, isSaving]
 */
export function useDebouncedConfig(
  configKey: string,
  configValue: string | undefined,
  updateConfig: UpdateConfigFn
): [string, React.Dispatch<React.SetStateAction<string>>, boolean] {
  const [localValue, setLocalValue] = useState('')
  const [saving, setSaving] = useState(false)
  const isEditingRef = useRef(false)
  const prevRef = useRef('')

  // Sync from engine config (only when not actively editing)
  useEffect(() => {
    if (configValue !== undefined && !isEditingRef.current && configValue !== prevRef.current) {
      setLocalValue(configValue)
      prevRef.current = configValue
    }
  }, [configValue])

  // Debounced save
  useEffect(() => {
    if (!localValue || configValue === undefined) return
    if (localValue === configValue) return

    isEditingRef.current = true
    const timer = setTimeout(async () => {
      setSaving(true)
      await updateConfig(configKey, localValue)
      setSaving(false)
      isEditingRef.current = false
      prevRef.current = localValue
    }, 1500)

    return () => clearTimeout(timer)
  }, [localValue, configValue, configKey, updateConfig])

  return [localValue, setLocalValue, saving]
}

/**
 * Hook for managing multiple debounced config fields.
 * Used when a component has several debounced inputs (e.g. AgentConfig).
 */
export function useDebouncedConfigMap(
  fields: Record<string, { configKey: string; configValue: string | undefined }>,
  updateConfig: UpdateConfigFn
): {
  values: Record<string, string>
  setValue: (key: string, value: string) => void
  savings: Record<string, boolean>
} {
  const [values, setValues] = useState<Record<string, string>>(() => {
    const init: Record<string, string> = {}
    for (const key of Object.keys(fields)) init[key] = ''
    return init
  })
  const [savings, setSavings] = useState<Record<string, boolean>>(() => {
    const init: Record<string, boolean> = {}
    for (const key of Object.keys(fields)) init[key] = false
    return init
  })
  const isEditingRefs = useRef<Record<string, boolean>>({})
  const prevRefs = useRef<Record<string, string>>({})

  const setValue = useCallback((key: string, value: string) => {
    setValues((prev) => ({ ...prev, [key]: value }))
  }, [])

  // Sync from engine config
  useEffect(() => {
    for (const [key, field] of Object.entries(fields)) {
      if (field.configValue !== undefined && !isEditingRefs.current[key] && field.configValue !== prevRefs.current[key]) {
        setValues((prev) => ({ ...prev, [key]: field.configValue! }))
        prevRefs.current[key] = field.configValue!
      }
    }
  }, [fields])

  // Debounced save for each field
  useEffect(() => {
    const timers: ReturnType<typeof setTimeout>[] = []

    for (const [key, field] of Object.entries(fields)) {
      const val = values[key]
      if (!val || field.configValue === undefined) continue
      if (val === field.configValue) continue

      isEditingRefs.current[key] = true
      const timer = setTimeout(async () => {
        setSavings((prev) => ({ ...prev, [key]: true }))
        await updateConfig(field.configKey, val)
        setSavings((prev) => ({ ...prev, [key]: false }))
        isEditingRefs.current[key] = false
        prevRefs.current[key] = val
      }, 1500)
      timers.push(timer)
    }

    return () => {
      for (const t of timers) clearTimeout(t)
    }
  }, [values, fields, updateConfig])

  return { values, setValue, savings }
}
