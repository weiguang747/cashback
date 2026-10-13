import { getCalendars, getLocales } from 'expo-localization';
import { AppState } from 'react-native';

type DeviceLocale = {
  /** BCP 47 tag of the user's preferred language, e.g. "id-ID". */
  languageTag: string;
  /** The user's 12/24-hour setting, or null to use the locale's default. */
  uses24hourClock: boolean | null;
  /** Currency of the device's region, e.g. "IDR", and its local symbol, e.g. "Rp". */
  currencyCode: string | null;
  currencySymbol: string | null;
};

function read(): DeviceLocale {
  const locale = getLocales()[0];
  return {
    languageTag: locale?.languageTag ?? 'en-US',
    uses24hourClock: getCalendars()[0]?.uses24hourClock ?? null,
    currencyCode: locale?.currencyCode ?? null,
    currencySymbol: locale?.currencySymbol ?? null,
  };
}

let current = read();

// Android can change the language without restarting the app, so re-read
// it whenever the app comes back to the foreground.
AppState.addEventListener('change', (state) => {
  if (state === 'active') current = read();
});

/** The device's locale settings, used by every formatter in the app. */
export function deviceLocale(): DeviceLocale {
  return current;
}

/**
 * "Good morning", "Good afternoon" or "Good evening" for the device's local
 * time. Display only, so the device clock is fine here (unlike cash back limits).
 */
export function greeting(now: Date = new Date()): string {
  const hour = now.getHours();
  if (hour >= 5 && hour < 12) return 'Good morning';
  if (hour >= 12 && hour < 18) return 'Good afternoon';
  return 'Good evening';
}
