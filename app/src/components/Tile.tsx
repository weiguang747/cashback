import type { ReactNode } from 'react';
import { StyleSheet, View } from 'react-native';

import { tones, typography, type Tone } from '@/theme';

import { Text } from './Text';

type TileProps = {
  tone?: Tone;
  size?: number;
  /** Corner radius; pass size / 2 for a circle. */
  radius?: number;
  /** A letter, or an icon via `children`. */
  label?: string;
  children?: ReactNode;
};

/** Tinted square (or circle) holding an initial or an icon. */
export function Tile({ tone = 'neutral', size = 44, radius = 14, label, children }: TileProps) {
  const t = tones[tone];
  return (
    <View
      style={[styles.tile, { width: size, height: size, borderRadius: radius, backgroundColor: t.bg }]}
      accessibilityElementsHidden
      importantForAccessibility="no-hide-descendants"
    >
      {children ?? (
        <Text style={[typography.bodyStrong, { color: t.fg }]}>{label?.charAt(0).toUpperCase()}</Text>
      )}
    </View>
  );
}

const styles = StyleSheet.create({
  tile: { alignItems: 'center', justifyContent: 'center' },
});
