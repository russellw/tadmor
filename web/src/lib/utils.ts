import { clsx, type ClassValue } from 'clsx'
import { twMerge } from 'tailwind-merge'

// Standard shadcn/ui class-name helper: merge conditional classes and resolve
// Tailwind conflicts (last-wins). Used by vendored components in components/ui.
export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs))
}
