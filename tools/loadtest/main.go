package main

import (
	"encoding/json"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"time"

	"github.com/gorilla/websocket"
)

type LobbyData struct {
	LobbyID                string `json:"lobbyId"`
	DrawingBoardBaseWidth  int    `json:"drawingBoardBaseWidth"`
	DrawingBoardBaseHeight int    `json:"drawingBoardBaseHeight"`
}

type GameEvent struct {
	Type string      `json:"type"`
	Data interface{} `json:"data"`
}

func waitForCommand(socket *websocket.Conn, command string) *GameEvent {
	log.Printf("wait for %s", command)
	var result *GameEvent

	result = nil
	for start := time.Now(); time.Since(start) < 5*time.Second; {
		messageType, data, err := socket.ReadMessage()
		if err != nil {
			log.Fatalf("Error on socket read: %s", err.Error())
		}
		log.Printf("Received: %d %s ", messageType, data)
		if messageType == websocket.TextMessage {
			received := &GameEvent{}
			errM := json.Unmarshal(data, received)
			if errM != nil {
				log.Fatalf("Error on unmarshall: %s", err.Error())
			}
			log.Printf("type %s", received.Type)
			if received.Type == command {
				log.Printf("received command %s", command)
				result = received
				break
			}
		}
	}
	return result
}

func main() {
	//This example doesn't do error handling!

	//Create new lobby. This will return the ID of the newly created lobby.
	r, _ := http.PostForm("http://localhost:8080/v1/lobby", url.Values{
		"username":             {"Marcel"},
		"language":             {"english"},
		"max_players":          {"24"},
		"drawing_time":         {"120"},
		"rounds":               {"5"},
		"custom_words_chance":  {"50"},
		"enable_votekick":      {"true"},
		"clients_per_ip_limit": {"10"},
		"public":               {"true"},
	})

	rawData, _ := ioutil.ReadAll(r.Body)
	lobbyData := &LobbyData{}
	json.Unmarshal(rawData, lobbyData)

	//The usersession gets stored in a cookie
	userSession := r.Cookies()[0].Value
	log.Println("Created lobby: " + lobbyData.LobbyID)
	log.Println("Usersession: " + userSession)

	//Connecting the socket by setting the Usersession and dialing.
	u := &url.URL{Scheme: "ws", Host: "localhost:8080", Path: "/v1/ws", RawQuery: "lobby_id=" + lobbyData.LobbyID}
	header := make(http.Header)
	header["Usersession"] = []string{userSession}
	socket, _, _ := websocket.DefaultDialer.Dial(u.String(), header)
	defer socket.Close()

	//Do stuff with connection ...
	data := map[string]interface{}{
		"type": "name-change",
		"data": "Guillaume?",
	}
	socket.WriteJSON(data)
	data = map[string]interface{}{
		"type": "start",
	}
	socket.WriteJSON(data)
	duration := 10
	go func(socket *websocket.Conn) {
		for {
			data := map[string]interface{}{
				"type": "keep-alive",
			}
			socket.WriteJSON(data)
			log.Printf("send keep-alive")
			time.Sleep(1 * time.Second)
			duration = duration - 1
			if duration == 0 {
				break
			}

		}
	}(socket)
	waitForCommand(socket, "ready")
	waitForCommand(socket, "update-players")
	waitForCommand(socket, "next-turn")
	waitForCommand(socket, "your-turn")
	data = map[string]interface{}{
		"type": "choose-word",
		"data": 0,
	}
	log.Printf("choose word")
	socket.WriteJSON(data)
	time.Sleep(60 * time.Second)
}
