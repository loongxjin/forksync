import { clsx, type ClassValue } from 'clsx'
import { twMerge } from 'tailwind-merge'
import type { RepoStatus } from '@/types/engine'

export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs))
}

/** Check if a repo status indicates a conflict-related state (conflict / resolving / resolved) */
export function isConflictStatus(status: RepoStatus): boolean {
  return status === 'conflict' || status === 'resolving' || status === 'resolved'
}
