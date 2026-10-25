export const STATUS_CONFIG = {
  MUNICIPAL: {
    label: 'Municipal supply',
    short: 'Piped',
    color: 'var(--color-municipal)',
    bg: 'var(--color-municipal-bg)',
    description: 'Reliable piped PMC connection reaches this area.',
  },
  TANKER_DEPENDENT: {
    label: 'Tanker-dependent',
    short: 'Tanker',
    color: 'var(--color-tanker)',
    bg: 'var(--color-tanker-bg)',
    description: 'No functioning piped connection; residents rely on tankers or borewells.',
  },
  MIXED: {
    label: 'Mixed coverage',
    short: 'Mixed',
    color: 'var(--color-mixed)',
    bg: 'var(--color-mixed-bg)',
    description: 'Partial piped coverage -- some streets or societies still depend on tankers.',
  },
  PIPELINE_IN_PROGRESS: {
    label: 'Pipeline work underway',
    short: 'In progress',
    color: 'var(--color-progress)',
    bg: 'var(--color-progress-bg)',
    description: 'Pipeline-laying or connection work is actively underway but not yet delivering water.',
  },
}

export const STATUS_ORDER = ['MUNICIPAL', 'MIXED', 'PIPELINE_IN_PROGRESS', 'TANKER_DEPENDENT']
