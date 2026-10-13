/**
 * Color tokens for the Cashback app.
 *
 * `palette` holds the raw values; `colors` maps them to semantic roles.
 * Components should only ever reference `colors`.
 */
const palette = {
  white: '#FFFFFF',

  // Neutrals (warm grey-green)
  sand50: '#F3F4EF',
  sand100: '#ECEDE7',
  sand200: '#E7E9E2',
  sand300: '#E4E6DF',
  sand400: '#DDE0D8',
  sand500: '#D3D7CE',
  slate300: '#9AA39D',
  slate400: '#6B756F',
  slate500: '#5B6660',
  slate600: '#3E4943',
  ink: '#0F1A14',

  // Brand (forest green)
  forest900: '#0F2A20',
  forest700: '#0B6E4F',
  forest600: '#08513A',
  forest100: '#D9E6DF',
  mist: '#B9CBC1',

  // Reward (gold)
  gold100: '#F6E7C1',
  gold700: '#6B4E17',
  gold900: '#3D2A08',

  // Danger
  red100: '#F5DEDA',
  red700: '#B3261E',
  red900: '#7A2A22',

  // Accent (navy)
  navy100: '#DCE3F2',
  navy900: '#1E2B4A',
} as const;

export const colors = {
  // Surfaces
  background: palette.sand50,
  surface: palette.white,
  surfaceMuted: palette.sand200,
  surfaceSubtle: palette.sand100,

  // Text
  text: palette.ink,
  textSecondary: palette.slate600,
  textMuted: palette.slate500,
  textDisabled: palette.slate400,
  textPlaceholder: palette.slate300,

  // Lines
  border: palette.sand400,
  borderStrong: palette.sand500,
  divider: palette.sand300,
  dividerSoft: palette.sand100,

  // Brand
  brand: palette.forest900,
  onBrand: palette.sand50,
  onBrandMuted: palette.mist,
  onBrandOverlay: 'rgba(243,244,239,0.08)',
  onBrandOverlayStrong: 'rgba(243,244,239,0.12)',
  onBrandBorder: 'rgba(243,244,239,0.14)',

  // Status
  positive: palette.forest700,
  positivePressed: palette.forest600,
  positiveSoft: palette.forest100,
  danger: palette.red700,
  dangerSoft: palette.red100,

  // Rewards / cash back
  reward: palette.gold100,
  onReward: palette.gold900,
  onRewardMuted: palette.gold700,
  rewardTrack: 'rgba(61,42,8,0.14)',

  // Disabled controls
  disabled: palette.sand400,
  onDisabled: palette.slate400,
} as const;

/** Tinted tiles used for avatars, merchants and transaction rows. */
export const tones = {
  neutral: { bg: palette.sand100, fg: palette.slate600 },
  positive: { bg: palette.forest100, fg: palette.forest700 },
  reward: { bg: palette.gold100, fg: palette.gold900 },
  navy: { bg: palette.navy100, fg: palette.navy900 },
  rose: { bg: palette.red100, fg: palette.red900 },
} as const;

export type Tone = keyof typeof tones;
export type ColorToken = keyof typeof colors;
export { palette };
