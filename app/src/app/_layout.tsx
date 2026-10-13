import { DMSans_400Regular, DMSans_500Medium, DMSans_600SemiBold } from '@expo-google-fonts/dm-sans';
import {
  SpaceGrotesk_500Medium,
  SpaceGrotesk_600SemiBold,
  SpaceGrotesk_700Bold,
} from '@expo-google-fonts/space-grotesk';
import { useFonts } from 'expo-font';
import { Stack } from 'expo-router';
import * as SplashScreen from 'expo-splash-screen';
import { StatusBar } from 'expo-status-bar';
import { useEffect } from 'react';

import { useUser } from '@/lib/session';
import { colors } from '@/theme';

SplashScreen.preventAutoHideAsync();

export default function RootLayout() {
  const user = useUser();
  const [loaded, error] = useFonts({
    DMSans_400Regular,
    DMSans_500Medium,
    DMSans_600SemiBold,
    SpaceGrotesk_500Medium,
    SpaceGrotesk_600SemiBold,
    SpaceGrotesk_700Bold,
  });

  useEffect(() => {
    if (loaded || error) SplashScreen.hideAsync();
  }, [loaded, error]);

  if (!loaded && !error) return null;

  return (
    <>
      <StatusBar style="dark" />
      <Stack screenOptions={{ headerShown: false, contentStyle: { backgroundColor: colors.background } }}>
        {/* Signing in or out flips these guards and the router redirects:
            to home (the first screen available) after sign-in, and back
            to sign-in after logout. */}
        <Stack.Protected guard={!user}>
          <Stack.Screen name="index" />
        </Stack.Protected>
        <Stack.Protected guard={!!user}>
          <Stack.Screen name="home" options={{ gestureEnabled: false }} />
          <Stack.Screen name="merchants" />
          <Stack.Screen name="pay" />
          <Stack.Screen name="payment-success" options={{ gestureEnabled: false }} />
          <Stack.Screen name="payment-failed" options={{ gestureEnabled: false }} />
          <Stack.Screen name="redeem" />
          <Stack.Screen name="transactions" />
          <Stack.Screen name="cashback" />
        </Stack.Protected>
      </Stack>
    </>
  );
}
