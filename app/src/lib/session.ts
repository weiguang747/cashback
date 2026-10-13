import { useSyncExternalStore } from 'react';

import type { User } from './api';

// The signed-in user, held in memory only: a full reload signs out.
let current: User | null = null;
const listeners = new Set<() => void>();

export function getUser(): User | null {
  return current;
}

export function setUser(user: User | null) {
  current = user;
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
