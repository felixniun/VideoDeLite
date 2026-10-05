/// <reference types="vite/client" />

declare module '*.vue' {
  import type { DefineComponent } from 'vue'
  const component: DefineComponent<{}, {}, any>
  export default component
}

interface Window {
  // Injected by Wails v2 at runtime
  go?: {
    main?: {
      App?: Record<string, (...args: any[]) => Promise<any>>
    }
  }
  runtime?: {
    EventsOn: (name: string, callback: (data: any) => void) => void
    EventsOff: (name: string) => void
    WindowMinimise: () => void
    WindowToggleMaximise: () => void
    WindowIsMaximised: () => Promise<boolean>
    Quit: () => void
  }
}
