// Wails bridge: works in the Wails webview (window.go / window.runtime are
// injected) and degrades gracefully in a plain browser for UI development.

function wailsBindings(): any | null {
  return (window as any).go?.main?.App ?? (window as any).go?.bindings?.main?.App ?? null
}

function wailsRuntime(): any | null {
  return (window as any).runtime ?? (window as any).go?.runtime ?? null
}

export const inWails = (): boolean => wailsBindings() !== null

export async function call<T>(method: string, ...args: any[]): Promise<T> {
  const b = wailsBindings()
  if (!b) throw new Error(`not in wails runtime: ${method}`)
  const fn = b[method]
  if (typeof fn !== 'function') throw new Error(`unknown binding: ${method}`)
  return fn(...args) as Promise<T>
}

export function onEvent(name: string, cb: (data: any) => void): void {
  const r = wailsRuntime()
  if (r?.EventsOn) r.EventsOn(name, cb)
}

export function offEvent(name: string): void {
  const r = wailsRuntime()
  if (r?.EventsOff) r.EventsOff(name)
}

export function minimise(): void { wailsRuntime()?.WindowMinimise?.() }
export function toggleMaximise(): void { wailsRuntime()?.WindowToggleMaximise?.() }
export function quit(): void { wailsRuntime()?.Quit?.() }
