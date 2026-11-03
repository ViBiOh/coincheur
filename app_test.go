package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestApp() (*App, *[]State) {
	app := NewApp()

	var states []State

	app.emit = func(event string, payload any) {
		if state, ok := payload.(State); ok && event == eventState {
			states = append(states, state)
		}
	}

	return app, &states
}

func TestDeck(t *testing.T) {
	t.Parallel()

	deck := NewApp().Deck()

	require.Len(t, deck, 32)
	assert.Equal(t, "AS", deck[0])
	assert.Equal(t, "7C", deck[31])
}

func TestScoresNeedATrump(t *testing.T) {
	t.Parallel()

	app, states := newTestApp()

	require.NoError(t, app.ToggleCard("JH"))
	require.NoError(t, app.ToggleCard("9H"))
	require.NoError(t, app.ToggleCard("AS"))

	assert.Nil(t, app.State().Scores)

	require.NoError(t, app.SetTrump("H"))
	require.NoError(t, app.SetLastTrick("other"))

	state := app.State()

	require.NotNil(t, state.Scores)
	assert.Equal(t, Scores{Scanned: 45, Other: 117}, *state.Scores)
	assert.Equal(t, []string{"AS", "JH", "9H"}, state.Cards)
	assert.NotEmpty(t, *states)
}

func TestBeloteAndLastTrick(t *testing.T) {
	t.Parallel()

	app, _ := newTestApp()

	require.NoError(t, app.ToggleCard("JH"))
	require.NoError(t, app.SetTrump("H"))
	require.NoError(t, app.SetBelote("scanned"))
	require.NoError(t, app.SetLastTrick("scanned"))

	assert.Equal(t, Scores{Scanned: 50, Other: 132}, *app.State().Scores)

	require.NoError(t, app.SetBelote(""))
	assert.Equal(t, Scores{Scanned: 30, Other: 132}, *app.State().Scores)
}

func TestCountWarning(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		cards []string
		want  string
	}{
		"no card":       {},
		"one trick":     {cards: []string{"AS", "KS", "QS", "JS"}},
		"one card":      {cards: []string{"AS"}, want: "1 cartes lues, un multiple de 4 est attendu (3 manquantes ou 1 en trop)."},
		"three cards":   {cards: []string{"AS", "KS", "QS"}, want: "3 cartes lues, un multiple de 4 est attendu (1 manquante ou 3 en trop)."},
		"two in excess": {cards: []string{"AS", "KS", "QS", "JS", "10S", "9S"}, want: "6 cartes lues, un multiple de 4 est attendu (2 manquantes ou 2 en trop)."},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			app, _ := newTestApp()

			for _, card := range test.cards {
				require.NoError(t, app.ToggleCard(card))
			}

			assert.Equal(t, test.want, app.State().CountWarning)
		})
	}
}

func TestInvalidInputs(t *testing.T) {
	t.Parallel()

	app, _ := newTestApp()

	require.Error(t, app.ToggleCard("2H"))
	require.Error(t, app.SetTrump("X"))
	require.Error(t, app.SetLastTrick("nobody"))
	require.Error(t, app.SetBelote("nobody"))
}

func TestNewRound(t *testing.T) {
	t.Parallel()

	app, _ := newTestApp()

	require.NoError(t, app.ToggleCard("AS"))
	require.NoError(t, app.SetTrump("S"))
	require.NoError(t, app.SetBelote("other"))
	require.NoError(t, app.SetLastTrick("other"))

	app.NewRound()

	state := app.State()

	assert.Empty(t, state.Cards)
	assert.Empty(t, state.Trump)
	assert.Empty(t, state.Belote)
	assert.Equal(t, "scanned", state.LastTrick)
	assert.Nil(t, state.Scores)
}

func TestNewRoundCanBeCancelled(t *testing.T) {
	t.Parallel()

	app, _ := newTestApp()
	app.confirm = func(string) bool { return false }

	require.NoError(t, app.ToggleCard("AS"))
	require.NoError(t, app.SetTrump("S"))

	app.NewRound()

	state := app.State()

	assert.Equal(t, []string{"AS"}, state.Cards)
	assert.Equal(t, "S", state.Trump)
}
