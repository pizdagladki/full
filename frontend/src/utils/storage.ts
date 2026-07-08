// Safe localStorage access: the vitest jsdom environment exposes a `localStorage`
// object WITHOUT the Storage methods, and real browsers can throw on access in
// privacy modes — so both directions are guarded and degrade to a no-op.
export function readLocal(key: string): string | null {
  try {
    if (typeof localStorage?.getItem === 'function') return localStorage.getItem(key);
  } catch {
    // storage unavailable — behave as "nothing saved"
  }
  return null;
}

export function writeLocal(key: string, value: string): void {
  try {
    if (typeof localStorage?.setItem === 'function') localStorage.setItem(key, value);
  } catch {
    // storage unavailable — persisting is best-effort
  }
}
