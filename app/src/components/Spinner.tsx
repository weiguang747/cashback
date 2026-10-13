import { useEffect } from 'react';
import Animated, {
  Easing,
  cancelAnimation,
  useAnimatedStyle,
  useSharedValue,
  withRepeat,
  withTiming,
} from 'react-native-reanimated';

import { Icon } from './Icon';

export function Spinner({ size = 20, color }: { size?: number; color?: string }) {
  const rotation = useSharedValue(0);

  useEffect(() => {
    rotation.value = withRepeat(withTiming(360, { duration: 800, easing: Easing.linear }), -1);
    return () => cancelAnimation(rotation);
  }, [rotation]);

  const style = useAnimatedStyle(() => ({ transform: [{ rotate: `${rotation.value}deg` }] }));

  return (
    // Fixed size, so it spins on its own centre even in a parent that stretches it.
    <Animated.View style={[{ width: size, height: size }, style]}>
      <Icon name="spinner" size={size} color={color} strokeWidth={2.4} />
    </Animated.View>
  );
}
