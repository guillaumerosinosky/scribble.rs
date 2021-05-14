package game

import (
	"encoding/json"
	"log"
	"sync"
	"testing"
)

func Test_marshallLobby(t *testing.T) {
	t.Run("test marshalling a simple lobby", func(t *testing.T) {
		owner := &Player{
			userSession:      "test",
			lastKnownAddress: "lastKnown",
			ID:               "id",
		}
		lobby := &Lobby{
			Owner:       owner,
			creator:     owner,
			CurrentWord: "",
			words:       []string{"a", "b", "c"},
			EditableLobbySettings: &EditableLobbySettings{
				CustomWordsChance: 0,
			},

			CustomWords: []string{"d", "e", "f"},
			mutex:       &sync.Mutex{},
		}
		lobby.players = append(lobby.players, &Player{
			ID:        "a",
			Score:     1,
			Connected: true,
		})
		lobby.players = append(lobby.players, &Player{
			ID:        "b",
			Score:     1,
			Connected: true,
		})
		line1 := &Line{
			FromX:     1,
			FromY:     2,
			ToX:       3,
			ToY:       4,
			Color:     RGBColor{R: 255, G: 127, B: 0},
			LineWidth: 1,
		}
		line2 := &Line{
			FromX:     4,
			FromY:     3,
			ToX:       2,
			ToY:       1,
			Color:     RGBColor{R: 255, G: 127, B: 0},
			LineWidth: 1,
		}

		lineEvent1 := &LineEvent{
			Type: "line",
			Data: line1,
		}
		lineEvent2 := &LineEvent{
			Type: "line",
			Data: line2,
		}
		lobby.AppendLine(lineEvent1)
		lobby.AppendLine(lineEvent2)

		m := MarshallLobby(lobby)

		result, err := json.Marshal(m)
		if err != nil {
			log.Fatal(err)
		}
		log.Printf("%s", string(result))

	})
}

const lobby1 = "{\"LobbyID\":\"\",\"EditableLobbySettings\":{\"maxPlayers\":0,\"public\":false,\"enableVotekick\":false,\"customWordsChance\":0,\"clientsPerIpLimit\":0,\"drawingTime\":0,\"rounds\":0},\"DrawingTimeNew\":0,\"CustomWords\":[\"d\",\"e\",\"f\"],\"Words\":[\"a\",\"b\",\"c\"],\"Players\":[{\"UserSession\":\"\",\"LastKnownAddress\":\"\",\"DisconnectTime\":null,\"VotedForKick\":null,\"ID\":\"a\",\"Name\":\"\",\"Score\":1,\"Connected\":true,\"LastScore\":0,\"Rank\":0,\"State\":\"\"},{\"UserSession\":\"\",\"LastKnownAddress\":\"\",\"DisconnectTime\":null,\"VotedForKick\":null,\"ID\":\"b\",\"Name\":\"\",\"Score\":1,\"Connected\":true,\"LastScore\":0,\"Rank\":0,\"State\":\"\"}],\"State\":\"\",\"Owner\":{\"UserSession\":\"test\",\"LastKnownAddress\":\"lastKnown\",\"DisconnectTime\":null,\"VotedForKick\":null,\"ID\":\"id\",\"Name\":\"\",\"Score\":0,\"Connected\":false,\"LastScore\":0,\"Rank\":0,\"State\":\"\"},\"Creator\":{\"UserSession\":\"test\",\"LastKnownAddress\":\"lastKnown\",\"DisconnectTime\":null,\"VotedForKick\":null,\"ID\":\"id\",\"Name\":\"\",\"Score\":0,\"Connected\":false,\"LastScore\":0,\"Rank\":0,\"State\":\"\"},\"CurrentWord\":\"\",\"WordHints\":null,\"WordHintsShown\":null,\"HintsLeft\":0,\"HintCount\":0,\"Round\":0,\"WordChoice\":null,\"Wordpack\":\"\",\"RoundEndTime\":0,\"TimeLeftTicker\":null,\"ScoreEarnedByGuessers\":0,\"CurrentDrawing\":[{\"type\":\"line\",\"data\":{\"fromX\":1,\"fromY\":2,\"toX\":3,\"toY\":4,\"color\":{\"r\":255,\"g\":127,\"b\":0},\"lineWidth\":1}},{\"type\":\"line\",\"data\":{\"fromX\":4,\"fromY\":3,\"toX\":2,\"toY\":1,\"color\":{\"r\":255,\"g\":127,\"b\":0},\"lineWidth\":1}}],\"Lowercaser\":{},\"LastPlayerDisconnectTime\":null}"

func Test_unmarshallLobby(t *testing.T) {
	t.Run("test unmarshalling a simple lobby", func(t *testing.T) {
		var l LobbyEntity
		err := json.Unmarshal([]byte(lobby1), &l)
		if err != nil {
			log.Fatal(err)
		}
		lobby := UnmarshallLobby(l)
		log.Printf("\n %s", lobby.LobbyID)
		m := MarshallLobby(lobby)
		result, err := json.Marshal(m)
		if err != nil {
			log.Fatal(err)
		}
		log.Printf("%s", string(result))
		if lobby1 != string(result) {
			log.Printf("%s", string(result))
			log.Fatalf("Marshalled not the same.")
		}
	})
}
