import type { TextStyle } from 'react-native';

/**
 * Font families. DM Sans is the UI face; Space Grotesk is used for money and
 * other figures. Custom fonts on native need one family per weight, so weights
 * are picked by family name rather than `fontWeight`.
 */
export const fonts = {
  regular: 'DMSans_400Regular',
  medium: 'DMSans_500Medium',
  semibold: 'DMSans_600SemiBold',
  numericMedium: 'SpaceGrotesk_500Medium',
  numericSemibold: 'SpaceGrotesk_600SemiBold',
  numericBold: 'SpaceGrotesk_700Bold',
} as const;

export const typography = {
  /** Screen-level hero heading, e.g. "Sign in". */
  display: { fontFamily: fonts.semibold, fontSize: 30, lineHeight: 36, letterSpacing: -0.3 },
  /** Result screens: "Payment successful". */
  title: { fontFamily: fonts.semibold, fontSize: 22, lineHeight: 28 },
  /** Section headings: "Transactions". */
  heading: { fontFamily: fonts.semibold, fontSize: 18, lineHeight: 24 },
  /** Navigation bar titles and names. */
  headline: { fontFamily: fonts.semibold, fontSize: 17, lineHeight: 22 },
  bodyStrong: { fontFamily: fonts.semibold, fontSize: 15, lineHeight: 20 },
  body: { fontFamily: fonts.regular, fontSize: 15, lineHeight: 22 },
  label: { fontFamily: fonts.semibold, fontSize: 14, lineHeight: 18 },
  bodySmall: { fontFamily: fonts.regular, fontSize: 14, lineHeight: 20 },
  linkSmall: { fontFamily: fonts.medium, fontSize: 14, lineHeight: 18 },
  caption: { fontFamily: fonts.regular, fontSize: 13, lineHeight: 18 },
  captionStrong: { fontFamily: fonts.medium, fontSize: 13, lineHeight: 18 },
  micro: { fontFamily: fonts.medium, fontSize: 12, lineHeight: 16 },
  microStrong: { fontFamily: fonts.semibold, fontSize: 12, lineHeight: 16 },
  overline: {
    fontFamily: fonts.semibold,
    fontSize: 13,
    lineHeight: 18,
    letterSpacing: 0.26,
    textTransform: 'uppercase',
  },
  button: { fontFamily: fonts.semibold, fontSize: 17, lineHeight: 22 },
  buttonSmall: { fontFamily: fonts.semibold, fontSize: 15, lineHeight: 20 },

  // Figures
  numeric: { fontFamily: fonts.numericSemibold, fontSize: 16, lineHeight: 20 },
  numericSmall: { fontFamily: fonts.numericSemibold, fontSize: 14, lineHeight: 18 },
  keypad: { fontFamily: fonts.numericMedium, fontSize: 28, lineHeight: 34 },
} satisfies Record<string, TextStyle>;

export type TypographyVariant = keyof typeof typography;

/** Size presets for <Amount />: currency label + figure. */
export const amountSizes = {
  xl: { figure: 64, currency: 28, gap: 8 },
  lg: { figure: 52, currency: 28, gap: 8 },
  md: { figure: 48, currency: 24, gap: 8 },
  sm: { figure: 44, currency: 24, gap: 6 },
} as const;

export type AmountSize = keyof typeof amountSizes;
