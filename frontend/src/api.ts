export interface Scores {
  scanned: number
  other: number
}

export interface State {
  cards: string[]
  trump: string
  lastTrick: string
  belote: string
  scores: Scores | null
  countWarning: string
  scanning: boolean
  status: string
}

export interface BoxPayload {
  card: string
  score: number
  x0: number
  y0: number
  x1: number
  y1: number
}

export interface FramePayload {
  preview: string
  boxes: BoxPayload[]
  inferenceMs: number
}

interface AppBindings {
  Start(): Promise<void>
  Stop(): Promise<void>
  State(): Promise<State>
  Deck(): Promise<string[]>
  ToggleCard(card: string): Promise<void>
  SetTrump(suit: string): Promise<void>
  SetLastTrick(holder: string): Promise<void>
  SetBelote(holder: string): Promise<void>
  NewRound(): Promise<void>
  ReportError(): Promise<void>
  OpenReports(): Promise<void>
}

declare global {
  interface Window {
    go: { main: { App: AppBindings } }
    runtime: { EventsOn(name: string, callback: (payload: never) => void): () => void }
  }
}

export const app = (): AppBindings => window.go.main.App

export function onEvent<T>(name: string, callback: (payload: T) => void): void {
  window.runtime.EventsOn(name, callback as (payload: never) => void)
}
