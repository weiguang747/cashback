export * from './colors';
export * from './typography';

/** 4-pt spacing scale. */
export const spacing = {
  xxs: 2,
  xs: 4,
  sm: 8,
  md: 12,
  lg: 16,
  xl: 20,
  xxl: 24,
  xxxl: 32,
  /** Horizontal page gutter. */
  gutter: 24,
} as const;

export const radii = {
  sm: 12,
  md: 14,
  lg: 16,
  xl: 18,
  xxl: 20,
  card: 22,
  hero: 28,
  pill: 999,
} as const;

/** Minimum touch target and common control heights. */
export const sizes = {
  touch: 44,
  buttonLarge: 56,
  buttonMedium: 52,
  input: 56,
  search: 48,
  chip: 36,
  keypadKey: 64,
  tile: 44,
} as const;
