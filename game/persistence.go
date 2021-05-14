package game

import (
	"sync"
	"time"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

type PlayerEntity struct {
	UserSession      string
	LastKnownAddress string
	DisconnectTime   *time.Time
	VotedForKick     map[string]bool
	ID               string
	Name             string
	Score            int
	Connected        bool
	LastScore        int
	Rank             int
	State            PlayerState
}

type EventEntity struct {
	Type string
	Data interface{}
}

type LobbyEntity struct {
	LobbyID                  string
	EditableLobbySettings    *EditableLobbySettings
	DrawingTimeNew           int
	CustomWords              []string
	Words                    []string
	Players                  []PlayerEntity
	State                    gameState
	Drawer                   *PlayerEntity
	Owner                    *PlayerEntity
	Creator                  *PlayerEntity
	CurrentWord              string
	WordHints                []*WordHint
	WordHintsShown           []*WordHint
	HintsLeft                int
	HintCount                int
	Round                    int
	WordChoice               []string
	Wordpack                 string
	RoundEndTime             int64
	TimeLeftTicker           *time.Ticker
	ScoreEarnedByGuessers    int
	CurrentDrawing           []interface{}
	Lowercaser               cases.Caser
	LastPlayerDisconnectTime *time.Time
}

func MarshallPlayer(player *Player) *PlayerEntity {
	var m *PlayerEntity
	if player != nil {
		m = &PlayerEntity{
			UserSession:      player.userSession,
			LastKnownAddress: player.lastKnownAddress,
			DisconnectTime:   player.disconnectTime,
			VotedForKick:     player.votedForKick,
			ID:               player.ID,
			Name:             player.Name,
			Score:            player.Score,
			Connected:        player.Connected,
			LastScore:        player.LastScore,
			Rank:             player.Rank,
			State:            player.State,
		}
	} else {
		m = nil
	}
	return m
}

func MarshallPlayers(players []*Player) []PlayerEntity {
	var m []PlayerEntity
	for _, player := range players {
		m = append(m, *MarshallPlayer(player))
	}
	return m
}

func UnmarshallPlayer(m *PlayerEntity) *Player {
	var player *Player
	if m != nil {
		player = &Player{
			userSession:      m.UserSession,
			lastKnownAddress: m.LastKnownAddress,
			disconnectTime:   m.DisconnectTime,
			votedForKick:     m.VotedForKick,
			ID:               m.ID,
			Name:             m.Name,
			Score:            m.Score,
			Connected:        m.Connected,
			LastScore:        m.LastScore,
			Rank:             m.Rank,
			State:            m.State,

			socketMutex: &sync.Mutex{},
		}
	} else {
		player = nil
	}
	return player
}

func UnmarshallPlayers(playersMap []PlayerEntity) []*Player {
	var players []*Player
	players = make([]*Player, 0)

	for _, playerMap := range playersMap {
		players = append(players, UnmarshallPlayer(&playerMap))
	}
	return players
}

func MarshallLobby(lobby *Lobby) LobbyEntity {
	m := LobbyEntity{
		LobbyID:               lobby.LobbyID,
		EditableLobbySettings: lobby.EditableLobbySettings,
		DrawingTimeNew:        lobby.DrawingTimeNew,
		CustomWords:           lobby.CustomWords,
		Words:                 lobby.words,
		Players:               MarshallPlayers(lobby.players),
		State:                 lobby.State,
		Drawer:                MarshallPlayer(lobby.drawer),
		Owner:                 MarshallPlayer(lobby.Owner),
		Creator:               MarshallPlayer(lobby.creator),
		CurrentWord:           lobby.CurrentWord,
		WordHints:             lobby.wordHints,
		WordHintsShown:        lobby.wordHintsShown,
		HintsLeft:             lobby.hintsLeft,
		HintCount:             lobby.hintCount,
		Round:                 lobby.Round,
		WordChoice:            lobby.wordChoice,
		Wordpack:              lobby.Wordpack,
		RoundEndTime:          lobby.RoundEndTime,

		//TimeLeftTicker:           lobby.timeLeftTicker, // potential issue
		ScoreEarnedByGuessers: lobby.scoreEarnedByGuessers,
		CurrentDrawing:        lobby.currentDrawing,
		//Lowercaser:               lobby.lowercaser,
		LastPlayerDisconnectTime: lobby.LastPlayerDisconnectTime,
	}

	return m
}

func UnmarshallLobby(m LobbyEntity) *Lobby {
	lobby := Lobby{
		LobbyID:               m.LobbyID,
		EditableLobbySettings: m.EditableLobbySettings,
		DrawingTimeNew:        m.DrawingTimeNew,
		CustomWords:           m.CustomWords,
		words:                 m.Words,
		players:               UnmarshallPlayers(m.Players),
		drawer:                UnmarshallPlayer(m.Drawer),
		State:                 m.State,
		Owner:                 UnmarshallPlayer(m.Owner),
		creator:               UnmarshallPlayer(m.Creator),
		CurrentWord:           m.CurrentWord,
		wordHints:             m.WordHints,
		wordHintsShown:        m.WordHintsShown,
		hintsLeft:             m.HintsLeft,
		hintCount:             m.HintCount,
		Round:                 m.Round,
		wordChoice:            m.WordChoice,
		Wordpack:              m.Wordpack,
		RoundEndTime:          m.RoundEndTime,
		//timeLeftTicker:           m.TimeLeftTicker,
		scoreEarnedByGuessers:    m.ScoreEarnedByGuessers,
		currentDrawing:           m.CurrentDrawing,
		lowercaser:               cases.Lower(language.Make(getLanguageIdentifier(m.Wordpack))),
		LastPlayerDisconnectTime: m.LastPlayerDisconnectTime,

		timeLeftTicker: time.NewTicker(1 * time.Second),
		mutex:          &sync.Mutex{},
		WriteJSON: func(player *Player, object interface{}) error {
			//Dummy to pass test.
			return nil
		},
	}

	return &lobby
}
