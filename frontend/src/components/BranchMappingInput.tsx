/**
 * BranchMappingInput — shared branch mapping selector
 *
 * Renders a local→remote branch mapping with optional enable toggle.
 * Adapts between <select> (when branches are available) and <Input>
 * (free-form text) for each side.
 */

import { useTranslation } from 'react-i18next'
import { ArrowRight } from 'lucide-react'
import { Label } from './ui/label'
import { Input } from './ui/input'
import type { BranchMapping } from '@shared/types/engine'

// ---------------------------------------------------------------------------
// Props
// ---------------------------------------------------------------------------

interface BranchMappingInputProps {
  /** Current mapping value */
  value: BranchMapping | undefined
  /** Called when either field changes */
  onChange: (value: BranchMapping) => void
  /** Available local branches (empty → text input) */
  localBranches?: string[]
  /** Available remote branches (empty → text input) */
  remoteBranches?: string[]
  /** Compact layout (for ScanDialog) vs default (for AddRepoDialog) */
  compact?: boolean
}

// ---------------------------------------------------------------------------
// Component
// ---------------------------------------------------------------------------

export function BranchMappingInput({
  value,
  onChange,
  localBranches,
  remoteBranches,
  compact = false
}: BranchMappingInputProps): JSX.Element {
  const { t } = useTranslation()

  const updateField = (field: keyof BranchMapping, fieldValue: string): void => {
    onChange({
      localBranch: field === 'localBranch' ? fieldValue : (value?.localBranch || ''),
      remoteBranch: field === 'remoteBranch' ? fieldValue : (value?.remoteBranch || '')
    })
  }

  const sizeClasses = compact ? 'h-7 px-2 text-xs' : 'h-8 px-2 text-sm'

  return (
    <div className={`flex items-center gap-${compact ? '2' : '3'} p-${compact ? '2' : '3'} rounded${compact ? '' : '-md'} border border-border bg-background${compact ? '' : '/50'}`}>
      <div className="flex-1">
        {!compact && <Label className="text-xs">{t('addRepo.localBranch')}</Label>}
        {localBranches && localBranches.length > 0 ? (
          <select
            value={value?.localBranch || ''}
            onChange={(e) => updateField('localBranch', e.target.value)}
            className={`w-full ${sizeClasses} rounded${compact ? '' : '-md'} border border-input bg-background`}
          >
            <option value="">{compact ? t('scanRepo.localBranch') : t('common.select')}</option>
            {localBranches.map(branch => (
              <option key={branch} value={branch}>{branch}</option>
            ))}
          </select>
        ) : (
          <Input
            placeholder={compact ? t('scanRepo.localBranchPlaceholder') : t('addRepo.localPlaceholder')}
            value={value?.localBranch || ''}
            onChange={(e) => updateField('localBranch', e.target.value)}
            className={`${sizeClasses}`}
          />
        )}
      </div>

      <ArrowRight size={compact ? 12 : 16} className="text-muted-foreground" />

      <div className="flex-1">
        {!compact && <Label className="text-xs">{t('addRepo.remoteBranch')}</Label>}
        {remoteBranches && remoteBranches.length > 0 ? (
          <select
            value={value?.remoteBranch || ''}
            onChange={(e) => updateField('remoteBranch', e.target.value)}
            className={`w-full ${sizeClasses} rounded${compact ? '' : '-md'} border border-input bg-background`}
          >
            <option value="">{compact ? t('scanRepo.remoteBranch') : t('common.select')}</option>
            {remoteBranches.map(branch => (
              <option key={branch} value={branch}>{branch}</option>
            ))}
          </select>
        ) : (
          <Input
            placeholder={compact ? t('scanRepo.remoteBranchPlaceholder') : t('addRepo.remotePlaceholder')}
            value={value?.remoteBranch || ''}
            onChange={(e) => updateField('remoteBranch', e.target.value)}
            className={`${sizeClasses}`}
          />
        )}
      </div>
    </div>
  )
}
