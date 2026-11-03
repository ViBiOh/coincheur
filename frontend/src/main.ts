import './style.css'
import { BoxPayload, FramePayload, State, app, onEvent } from './api'

const SUITS = ['S', 'H', 'D', 'C'] as const
const SUIT_SYMBOLS: Record<string, string> = { S: '♠', H: '♥', D: '♦', C: '♣' }
const SUIT_NAMES: Record<string, string> = { S: 'Pique', H: 'Coeur', D: 'Carreau', C: 'Trèfle' }

let state: State = {
  cards: [],
  trump: '',
  lastTrick: 'scanned',
  belote: '',
  scores: null,
  countWarning: '',
  scanning: false,
  status: '',
}
let deck: string[] = []
let lastBoxes: BoxPayload[] = []

const root = document.querySelector<HTMLElement>('#app')!

const preview = document.createElement('img')
preview.alt = ''
const overlay = document.createElement('canvas')
const frame = document.createElement('div')
frame.className = 'video-frame'
frame.append(preview, overlay)

const scanButton = document.createElement('button')
scanButton.className = 'primary'
scanButton.addEventListener('click', () => {
  void (state.scanning ? app().Stop() : app().Start().catch(() => undefined))
})

const reportButton = document.createElement('button')
reportButton.className = 'danger'
reportButton.textContent = 'Signaler une erreur'
reportButton.addEventListener('click', () => void app().ReportError().catch(() => undefined))

const reportsButton = document.createElement('button')
reportsButton.className = 'warn'
reportsButton.textContent = 'Ouvrir le dossier des erreurs'
reportsButton.addEventListener('click', () => void app().OpenReports().catch(() => undefined))

const newRoundButton = document.createElement('button')
newRoundButton.textContent = 'Nouvelle manche'
newRoundButton.addEventListener('click', () => void app().NewRound())

const actions = document.createElement('div')
actions.className = 'row actions'
actions.append(scanButton)

const errorActions = document.createElement('div')
errorActions.className = 'row error-actions'
errorActions.append(reportButton, reportsButton)

const roundActions = document.createElement('div')
roundActions.className = 'row actions'
roundActions.append(newRoundButton)

const statusLine = document.createElement('p')
statusLine.className = 'status'
statusLine.setAttribute('aria-live', 'polite')

const videoPanel = document.createElement('div')
videoPanel.className = 'video-panel'
videoPanel.append(frame, actions, roundActions, statusLine, errorActions)

const cardsPanel = document.createElement('div')
const controls = document.createElement('div')
controls.className = 'controls'

const sidePanel = document.createElement('div')
sidePanel.className = 'side-panel'
sidePanel.append(cardsPanel, controls)

const split = document.createElement('div')
split.className = 'split'
split.append(videoPanel, sidePanel)
root.append(split)

function isRed(value: string): boolean {
  return value === 'H' || value === 'D'
}

function label(card: string): string {
  return `${card.slice(0, -1)}${SUIT_SYMBOLS[card.slice(-1)]}`
}

function section(title: string, ...content: HTMLElement[]): HTMLElement {
  const wrapper = document.createElement('section')
  const heading = document.createElement('h2')
  heading.textContent = title
  wrapper.append(heading, ...content)
  return wrapper
}

function subtitle(text: string): HTMLElement {
  const heading = document.createElement('h3')
  heading.textContent = text
  return heading
}

function choices(
  options: { value: string; label: string; red?: boolean }[],
  current: string,
  onPick: (value: string) => void,
): HTMLElement {
  const row = document.createElement('div')
  row.className = 'row'
  for (const option of options) {
    const button = document.createElement('button')
    button.textContent = option.label
    button.classList.toggle('active', option.value === current)
    button.classList.toggle('red', option.red === true)
    button.setAttribute('aria-pressed', String(option.value === current))
    button.addEventListener('click', () => onPick(option.value))
    row.append(button)
  }
  return row
}

function cardsSection(): HTMLElement {
  const grid = document.createElement('div')
  for (const suit of SUITS) {
    const row = document.createElement('div')
    row.className = 'suit-row'
    for (const card of deck.filter((name) => name.endsWith(suit))) {
      const button = document.createElement('button')
      button.textContent = label(card)
      button.classList.toggle('red', isRed(suit))
      button.classList.toggle('active', state.cards.includes(card))
      button.setAttribute('aria-pressed', String(state.cards.includes(card)))
      button.addEventListener('click', () => void app().ToggleCard(card))
      row.append(button)
    }
    grid.append(row)
  }
  return section(`Cartes vues (${state.cards.length}), touchez pour corriger`, grid)
}

function bonusSection(): HTMLElement {
  const holders = [
    { value: 'scanned', label: 'Équipe scannée' },
    { value: 'other', label: 'Autre équipe' },
  ]
  const lastTrick = document.createElement('div')
  lastTrick.append(
    subtitle('10 de der'),
    choices(holders, state.lastTrick, (value) => void app().SetLastTrick(value)),
  )
  const belote = document.createElement('div')
  belote.append(
    subtitle('Belote et rebelote'),
    choices([{ value: '', label: 'Aucune' }, ...holders], state.belote, (value) => void app().SetBelote(value)),
  )
  return section('Bonus', lastTrick, belote)
}

function resultSection(): HTMLElement {
  const wrapper = document.createElement('div')
  if (!state.scores) {
    wrapper.textContent = 'Choisissez l’atout pour voir les points.'
    return section('Résultat', wrapper)
  }

  wrapper.className = 'scores'
  for (const [name, points] of [
    ['Équipe scannée', state.scores.scanned],
    ['Autre équipe', state.scores.other],
  ] as const) {
    const box = document.createElement('div')
    box.className = 'score'
    const value = document.createElement('strong')
    value.textContent = String(points)
    box.append(name, value)
    wrapper.append(box)
  }

  if (!state.countWarning) {
    return section('Résultat', wrapper)
  }

  const warning = document.createElement('p')
  warning.className = 'warning'
  warning.setAttribute('role', 'alert')
  warning.textContent = state.countWarning
  return section('Résultat', warning, wrapper)
}

function renderPanel(): void {
  scanButton.textContent = state.scanning ? 'Arrêter le scan' : 'Démarrer le scan'
  scanButton.classList.toggle('active', state.scanning)
  reportButton.disabled = !state.scanning
  statusLine.textContent = state.status
}

function render(): void {
  renderPanel()
  cardsPanel.replaceChildren(cardsSection())
  controls.replaceChildren(
    section(
      'Atout',
      choices(
        SUITS.map((suit) => ({ value: suit, label: `${SUIT_SYMBOLS[suit]} ${SUIT_NAMES[suit]}`, red: isRed(suit) })),
        state.trump,
        (value) => void app().SetTrump(value),
      ),
    ),
    bonusSection(),
    resultSection(),
  )
}

function drawBoxes(): void {
  overlay.width = preview.naturalWidth
  overlay.height = preview.naturalHeight
  const ctx = overlay.getContext('2d')!
  ctx.clearRect(0, 0, overlay.width, overlay.height)
  ctx.lineWidth = Math.max(2, overlay.width / 320)
  ctx.font = `bold ${Math.round(overlay.width / 28)}px system-ui`
  ctx.textBaseline = 'bottom'

  for (const box of lastBoxes) {
    const confirmed = state.cards.includes(box.card)
    ctx.strokeStyle = confirmed ? '#22aa77' : '#ff9800'
    ctx.fillStyle = ctx.strokeStyle
    const left = box.x0 * overlay.width
    const top = box.y0 * overlay.height
    ctx.strokeRect(left, top, (box.x1 - box.x0) * overlay.width, (box.y1 - box.y0) * overlay.height)
    ctx.fillText(label(box.card), left, Math.max(top - 2, ctx.lineWidth * 6))
  }
}

preview.addEventListener('load', drawBoxes)

onEvent<State>('state', (next) => {
  state = next
  render()
})

onEvent<FramePayload>('frame', (payload) => {
  lastBoxes = payload.boxes ?? []
  preview.src = `data:image/jpeg;base64,${payload.preview}`
  frame.classList.add('live')
})

async function start(): Promise<void> {
  deck = await app().Deck()
  state = await app().State()
  render()
}

void start()
