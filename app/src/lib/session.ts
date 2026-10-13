import * as SecureStore from 'expo-secure-store';
import { useSyncExternalStore } from 'react';
import { Platform } from 'react-native';

import type { User } from './api';
import { clearCache } from './cache';

// The signed-in user, kept in SecureStore (Keychain / Keystore) so reloads
// and restarts stay signed in. Web has no SecureStore, so there it lives in
// memory only.
const STORAGE_KEY = 'session.user';
const persisted = Platform.OS !== 'web';

let current: User | null = restore();
const listeners = new Set<() => void>();

// Synchronous so the first render already knows who is signed in: no
// loading state, and no flash of the sign-in screen.
function restore(): User | null {
  if (!persisted) return null;
  try {
    const raw = SecureStore.getItem(STORAGE_KEY);
    const user = raw ? JSON.parse(raw) : null;
    return isUser(user) ? user : null;
  } catch {
    return null;
  }
}

function isUser(v: unknown): v is User {
  const u = v as User | null;
  return typeof u?.id === 'string' && typeof u.email === 'string' && typeof u.name === 'string';
}

function save(user: User | null) {
  if (!persisted) return;
  const write = user ? SecureStore.setItemAsync(STORAGE_KEY, JSON.stringify(user)) : SecureStore.deleteItemAsync(STORAGE_KEY);
  // Losing the write only means signing in again next launch.
  write.catch(() => {});
}

export function getUser(): User | null {
  return current;
}

export function setUser(user: User | null) {
  // Cached data belongs to whoever was signed in before.
  if (user?.id !== current?.id) clearCache();
  current = user;
  save(user);
  listeners.forEach((l) => l());
}

export function clearUser() {
  setUser(null);
}

function subscribe(listener: () => void) {
  listeners.add(listener);
  return () => {
    listeners.delete(listener);
  };
}

/** The signed-in user; re-renders on sign-in and sign-out. */
export function useUser(): User | null {
  return useSyncExternalStore(subscribe, getUser);
}
