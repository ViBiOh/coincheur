package scoring

import (
	"fmt"
	"slices"
)

const (
	CardsPerTrick   = 4
	TotalCardPoints = 152
	lastTrickBonus  = 10
	beloteBonus     = 20
)

type Suit string

const (
	Spades   Suit = "S"
	Hearts   Suit = "H"
	Diamonds Suit = "D"
	Clubs    Suit = "C"
)

type Card string

type Holder string

const (
	NoHolder Holder = ""
	Scanned  Holder = "scanned"
	Other    Holder = "other"
)

var (
	suits = []Suit{Spades, Hearts, Diamonds, Clubs}
	ranks = []string{"A", "K", "Q", "J", "10", "9", "8", "7"}

	trumpPoints = map[string]int{"J": 20, "9": 14, "A": 11, "10": 10, "K": 4, "Q": 3, "8": 0, "7": 0}
	plainPoints = map[string]int{"A": 11, "10": 10, "K": 4, "Q": 3, "J": 2, "9": 0, "8": 0, "7": 0}
)

// Deck lists the 32 cards ordered by suit then rank, which is the class order of the detection model.
func Deck() []Card {
	deck := make([]Card, 0, len(suits)*len(ranks))

	for _, suit := range suits {
		for _, rank := range ranks {
			deck = append(deck, Card(rank+string(suit)))
		}
	}

	return deck
}

func ParseSuit(value string) (Suit, error) {
	suit := Suit(value)
	if !slices.Contains(suits, suit) {
		return "", fmt.Errorf("unknown suit %q", value)
	}

	return suit, nil
}

func ParseCard(value string) (Card, error) {
	card := Card(value)
	if !slices.Contains(Deck(), card) {
		return "", fmt.Errorf("unknown card %q", value)
	}

	return card, nil
}

func ParseHolder(value string) (Holder, error) {
	holder := Holder(value)
	if holder != NoHolder && holder != Scanned && holder != Other {
		return "", fmt.Errorf("unknown holder %q", value)
	}

	return holder, nil
}

func (c Card) Suit() Suit {
	return Suit(c[len(c)-1:])
}

func (c Card) Rank() string {
	return string(c[:len(c)-1])
}

func (c Card) Points(trump Suit) int {
	if c.Suit() == trump {
		return trumpPoints[c.Rank()]
	}

	return plainPoints[c.Rank()]
}

type Input struct {
	Trump     Suit
	Cards     []Card
	LastTrick Holder
	Belote    Holder
}

type Score struct {
	Scanned int
	Other   int
}

// Compute deduces the team that was not scanned from the 152 card points, so only one team has to be shown.
func Compute(input Input) Score {
	scannedPoints := 0
	for _, card := range input.Cards {
		scannedPoints += card.Points(input.Trump)
	}

	score := Score{Scanned: scannedPoints, Other: TotalCardPoints - scannedPoints}
	score.add(input.LastTrick, lastTrickBonus)
	score.add(input.Belote, beloteBonus)

	return score
}

func (s *Score) add(holder Holder, points int) {
	switch holder {
	case Scanned:
		s.Scanned += points
	case Other:
		s.Other += points
	case NoHolder:
	}
}

// CountIssue reports how many cards are in excess or missing for the count to be a whole number of tricks.
func CountIssue(count int) (extra, missing int) {
	extra = count % CardsPerTrick
	if extra == 0 {
		return 0, 0
	}

	return extra, CardsPerTrick - extra
}
