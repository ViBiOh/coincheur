package scan

import (
	"slices"
	"sync"

	"coincheur/internal/scoring"
)

// Tracker confirms a card once it has been seen on enough consecutive frames, which filters one frame errors.
type Tracker struct {
	mutex     sync.Mutex
	required  int
	streaks   map[scoring.Card]int
	confirmed map[scoring.Card]bool
	banned    map[scoring.Card]bool
}

func NewTracker(required int) *Tracker {
	return &Tracker{
		required:  max(1, required),
		streaks:   make(map[scoring.Card]int),
		confirmed: make(map[scoring.Card]bool),
		banned:    make(map[scoring.Card]bool),
	}
}

// Observe records the cards seen on a frame and returns those that just got confirmed.
func (t *Tracker) Observe(seen []scoring.Card) []scoring.Card {
	t.mutex.Lock()
	defer t.mutex.Unlock()

	for card := range t.streaks {
		if !slices.Contains(seen, card) {
			t.streaks[card]--
			if t.streaks[card] <= 0 {
				delete(t.streaks, card)
			}
		}
	}

	var added []scoring.Card

	for _, card := range seen {
		if t.banned[card] || t.confirmed[card] {
			continue
		}

		t.streaks[card]++

		if t.streaks[card] >= t.required {
			t.confirmed[card] = true
			delete(t.streaks, card)

			added = append(added, card)
		}
	}

	return added
}

// Toggle is a manual correction. A removed card stays removed even if the camera keeps seeing it.
func (t *Tracker) Toggle(card scoring.Card) {
	t.mutex.Lock()
	defer t.mutex.Unlock()

	delete(t.streaks, card)

	if t.confirmed[card] {
		delete(t.confirmed, card)
		t.banned[card] = true

		return
	}

	delete(t.banned, card)
	t.confirmed[card] = true
}

func (t *Tracker) Reset() {
	t.mutex.Lock()
	defer t.mutex.Unlock()

	clear(t.streaks)
	clear(t.confirmed)
	clear(t.banned)
}

// Cards lists the confirmed cards in deck order.
func (t *Tracker) Cards() []scoring.Card {
	t.mutex.Lock()
	defer t.mutex.Unlock()

	var cards []scoring.Card

	for _, card := range scoring.Deck() {
		if t.confirmed[card] {
			cards = append(cards, card)
		}
	}

	return cards
}
