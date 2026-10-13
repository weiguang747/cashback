import Svg, { Circle, Path, Rect } from 'react-native-svg';

import { colors } from '@/theme';

/** Stroke icons used across the app (24×24 grid). */
const glyphs = {
  'chevron-left': <Path d="m15 18-6-6 6-6" />,
  'chevron-right': <Path d="m9 18 6-6-6-6" />,
  'arrow-up-right': (
    <>
      <Path d="M7 17 17 7" />
      <Path d="M8 7h9v9" />
    </>
  ),
  percent: (
    <>
      <Path d="M19 5 5 19" />
      <Circle cx={6.5} cy={6.5} r={2.5} />
      <Circle cx={17.5} cy={17.5} r={2.5} />
    </>
  ),
  'log-out': (
    <>
      <Path d="M9 21H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h4" />
      <Path d="m16 17 5-5-5-5" />
      <Path d="M21 12H9" />
    </>
  ),
  search: (
    <>
      <Circle cx={11} cy={11} r={7} />
      <Path d="m20 20-3.5-3.5" />
    </>
  ),
  check: <Path d="M20 6 9 17l-5-5" />,
  close: (
    <>
      <Path d="M18 6 6 18" />
      <Path d="m6 6 12 12" />
    </>
  ),
  backspace: (
    <>
      <Path d="M21 4H8l-7 8 7 8h13a2 2 0 0 0 2-2V6a2 2 0 0 0-2-2z" />
      <Path d="m18 9-6 6" />
      <Path d="m12 9 6 6" />
    </>
  ),
  receipt: (
    <>
      <Path d="M6 3h12v18l-3-2-3 2-3-2-3 2z" />
      <Path d="M9 8h6" />
      <Path d="M9 12h6" />
    </>
  ),
  info: (
    <>
      <Circle cx={12} cy={12} r={9} />
      <Path d="M12 8h.01" />
      <Path d="M11 12h1v4h1" />
    </>
  ),
  alert: (
    <>
      <Circle cx={12} cy={12} r={9} />
      <Path d="M12 8v4" />
      <Path d="M12 16h.01" />
    </>
  ),
  clock: (
    <>
      <Circle cx={12} cy={12} r={9} />
      <Path d="M12 7v5l3 2" />
    </>
  ),
  shield: <Path d="M12 22s8-4 8-10V5l-8-3-8 3v7c0 6 8 10 8 10" />,
  wallet: (
    <>
      <Rect x={3} y={6} width={18} height={13} rx={2} />
      <Path d="M3 10h18" />
    </>
  ),
  card: (
    <>
      <Rect x={3} y={6} width={18} height={13} rx={2} />
      <Path d="M3 10h18" />
      <Path d="M7 15h3" />
    </>
  ),
  spinner: <Path d="M21 12a9 9 0 1 1-6.2-8.56" />,
} as const;

export type IconName = keyof typeof glyphs;

type IconProps = {
  name: IconName;
  size?: number;
  color?: string;
  strokeWidth?: number;
};

export function Icon({ name, size = 20, color = colors.text, strokeWidth = 2 }: IconProps) {
  return (
    <Svg
      width={size}
      height={size}
      viewBox="0 0 24 24"
      fill="none"
      stroke={color}
      strokeWidth={strokeWidth}
      strokeLinecap="round"
      strokeLinejoin="round"
      accessibilityElementsHidden
      importantForAccessibility="no-hide-descendants"
    >
      {glyphs[name]}
    </Svg>
  );
}
