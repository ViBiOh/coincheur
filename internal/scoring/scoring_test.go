package scoring

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDeck(t *testing.T) {
	t.Parallel()

	deck := Deck()

	require.Len(t, deck, 32)
	assert.Equal(t, Card("AS"), deck[0])
	assert.Equal(t, Card("7S"), deck[7])
	assert.Equal(t, Card("AH"), deck[8])
	assert.Equal(t, Card("7C"), deck[31])
}

func TestCardPoints(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		card  Card
		trump Suit
		want  int
	}{
		"trump jack":   {card: "JH", trump: Hearts, want: 20},
		"trump nine":   {card: "9H", trump: Hearts, want: 14},
		"trump ace":    {card: "AH", trump: Hearts, want: 11},
		"trump ten":    {card: "10H", trump: Hearts, want: 10},
		"plain jack":   {card: "JS", trump: Hearts, want: 2},
		"plain nine":   {card: "9S", trump: Hearts, want: 0},
		"plain ace":    {card: "AS", trump: Hearts, want: 11},
		"plain seven":  {card: "7D", trump: Hearts, want: 0},
		"trump eight":  {card: "8H", trump: Hearts, want: 0},
		"plain ten":    {card: "10C", trump: Spades, want: 10},
		"plain king":   {card: "KD", trump: Spades, want: 4},
		"plain queen":  {card: "QD", trump: Spades, want: 3},
		"trump king":   {card: "KS", trump: Spades, want: 4},
		"trump queen":  {card: "QS", trump: Spades, want: 3},
		"trump jack s": {card: "JS", trump: Spades, want: 20},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, test.want, test.card.Points(test.trump))
		})
	}
}

func TestDeckTotalsOneHundredFiftyTwo(t *testing.T) {
	t.Parallel()

	for _, trump := range suits {
		total := 0
		for _, card := range Deck() {
			total += card.Points(trump)
		}

		assert.Equal(t, TotalCardPoints, total, "trump %s", trump)
	}
}

func TestCompute(t *testing.T) {
	t.Parallel()

	cards := []Card{"JH", "9H", "AS"}

	tests := map[string]struct {
		input Input
		want  Score
	}{
		"other team takes the last trick": {
			input: Input{Trump: Hearts, Cards: cards, LastTrick: Other},
			want:  Score{Scanned: 45, Other: 117},
		},
		"scanned team has last trick and belote": {
			input: Input{Trump: Hearts, Cards: cards, LastTrick: Scanned, Belote: Scanned},
			want:  Score{Scanned: 75, Other: 107},
		},
		"belote for the other team": {
			input: Input{Trump: Spades, Cards: cards, LastTrick: Scanned, Belote: Other},
			want:  Score{Scanned: 23, Other: 159},
		},
		"nothing scanned": {
			input: Input{Trump: Spades, LastTrick: Other},
			want:  Score{Scanned: 0, Other: 162},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, test.want, Compute(test.input))
		})
	}
}

func TestCountIssue(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		count       int
		wantExtra   int
		wantMissing int
	}{
		"none":              {count: 0},
		"one trick":         {count: 4},
		"four tricks":       {count: 16},
		"two in excess":     {count: 10, wantExtra: 2, wantMissing: 2},
		"one in excess":     {count: 5, wantExtra: 1, wantMissing: 3},
		"three in excess":   {count: 7, wantExtra: 3, wantMissing: 1},
		"whole deck":        {count: 32},
		"whole deck plus 1": {count: 33, wantExtra: 1, wantMissing: 3},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			extra, missing := CountIssue(test.count)

			assert.Equal(t, test.wantExtra, extra)
			assert.Equal(t, test.wantMissing, missing)
		})
	}
}

func TestParse(t *testing.T) {
	t.Parallel()

	suit, err := ParseSuit("H")
	require.NoError(t, err)
	assert.Equal(t, Hearts, suit)

	_, err = ParseSuit("X")
	require.Error(t, err)

	card, err := ParseCard("10D")
	require.NoError(t, err)
	assert.Equal(t, Card("10D"), card)

	_, err = ParseCard("2D")
	require.Error(t, err)

	holder, err := ParseHolder("")
	require.NoError(t, err)
	assert.Equal(t, NoHolder, holder)

	_, err = ParseHolder("nobody")
	require.Error(t, err)
}
