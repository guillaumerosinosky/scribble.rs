package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"runtime/debug"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"go.opentelemetry.io/otel/trace"

	"github.com/guillaumerosinosky/scribble.rs/game"
	"github.com/guillaumerosinosky/scribble.rs/state"
)

var persist = func(lobby *game.Lobby) {

}

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin:     func(r *http.Request) bool { return true },
}

var channelIn chan []byte
var channelOut chan []byte

func wsEndpoint(w http.ResponseWriter, r *http.Request) {
	sessionCookie := GetUserSession(r)
	if sessionCookie == "" {
		//This issue can happen if you illegally request a websocket
		//connection without ever having had a usersession or your
		//client having deleted the usersession cookie.
		http.Error(w, "you don't have access to this lobby;usersession not set", http.StatusUnauthorized)
		return
	}

	lobby, lobbyError := GetLobby(r)
	if lobbyError != nil {
		http.Error(w, lobbyError.Error(), http.StatusNotFound)
		return
	}

	lobby.Synchronized(func() {
		lobby.WriteJSON = WriteJSON
		player := lobby.GetPlayer(sessionCookie)
		if player == nil {
			http.Error(w, "you don't have access to this lobby;usersession unknown", http.StatusUnauthorized)
			return
		}

		ws, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		log.Printf("%s(%s) has connected\n", player.Name, player.ID)

		player.SetWebsocket(ws)

		if state.PubSub {
			// create channels in and out
			channelIn = make(chan []byte)
			channelOut = make(chan []byte)
			if lobby.IsReferenceReplica() {
				go state.SubscribeRedis(lobby.LobbyID+"-in", channelIn)
			}
			go state.SubscribeRedis(lobby.LobbyID+"-out", channelOut)
			go pubSubIn()
			go pubSubOut()
		}

		// wait a bit for goroutines to initialize properly TODO: find better way
		time.Sleep(500 * time.Millisecond)
		lobby.OnPlayerConnectUnsynchronized(context.TODO(), player)

		ws.SetCloseHandler(func(code int, text string) error {
			lobby.OnPlayerDisconnect(context.TODO(), player)
			return nil
		})
		go wsListen(lobby, player, ws)
	})
}

func wsListen(lobby *game.Lobby, player *game.Player, socket *websocket.Conn) {
	//Workaround to prevent crash, since not all kind of
	//disconnect errors are cleanly caught by gorilla websockets.
	defer func() {
		err := recover()
		if err != nil {
			log.Printf("Error occurred in wsListen.\n\tError: %s\n\tPlayer: %s(%s)\nStack %s\n", err, player.Name, player.ID, string(debug.Stack()))
			lobby.OnPlayerDisconnect(context.TODO(), player)
		}
	}()

	switch state.PersistenceMode {
	case "NONE":
		persist = state.NoSaveLobby
	case "BASIC":
		persist = state.SaveLobby
	case "EVENTS":
		//TODO: implement
	}

	for {
		messageType, data, err := socket.ReadMessage()
		if err != nil {
			if websocket.IsCloseError(err) || websocket.IsUnexpectedCloseError(err) ||
				//This happens when the server closes the connection. It will cause 1000 retries followed by a panic.
				strings.Contains(err.Error(), "use of closed network connection") {
				//Make sure that the sockethandler is called
				lobby.OnPlayerDisconnect(context.TODO(), player)
				//If the error is fatal, we stop listening for more messages.
				return
			}

			log.Printf("Error reading from socket: %s\n", err)
			//If the error doesn't seem fatal we attempt listening for more messages.
			continue
		}

		if messageType == websocket.TextMessage {
			if state.PubSub {
				// PUB data
				var event state.PersistedEvent
				event.LobbyId = lobby.LobbyID
				event.PlayerId = player.ID
				event.Data = data

				realData, err := json.Marshal(event)
				if err == nil {
					// publish on redis
					state.PublishRedis(lobby.LobbyID+"-in", realData)

					//channelIn <- realData
				} else {
					log.Printf("wsListen: error while marshalling %s", err)
				}

			} else {
				// handle directly
				HandleEvent(lobby, player, data)
			}
		}
	}
}

func pubSubIn() {
	for {
		//var data []byte
		var event state.PersistedEvent
		data := <-channelIn
		err := json.Unmarshal(data, &event)
		if err != nil {
			log.Fatalf("pubSubIn: Error while unmarshal in %s", err)
		}
		log.Printf("pubSubIn: received %s", event)
		// find lobby
		var player *game.Player

		lobby := state.GetLobby(event.LobbyId)
		for _, p := range lobby.GetPlayers() {
			if p.ID == event.PlayerId {
				player = p
			}
		}
		if player == nil {
			log.Printf("pubSubIn: player %s not found", event.PlayerId)

			player = lobby.JoinPlayer("")
			player.ID = event.PlayerId
			// TODO: initialize player from player µs
			lobby.OnPlayerConnectUnsynchronized(context.TODO(), player)

		}
		HandleEvent(lobby, player, event.Data)
	}
}

func HandleEvent(lobby *game.Lobby, player *game.Player, data []byte) error {
	received := &game.GameEvent{}
	err := json.Unmarshal(data, received)
	if err != nil {
		log.Printf("Error unmarshalling message: %s\n", err)
		sendError := WriteJSON(context.TODO(), lobby, player, game.GameEvent{Type: "system-message", Data: fmt.Sprintf("An error occurred trying to read your request, please report the error via GitHub: %s!", err)})
		if sendError != nil {
			log.Printf("Error sending errormessage: %s\n", sendError)
		}
		return sendError
	}
	handleError := lobby.HandleEvent(data, received, player, persist)
	if handleError != nil {
		log.Printf("Error handling event: %s\n", handleError)
		return handleError
	}
	return nil
}

// WriteJSON marshals the given input into a JSON string and sends it to the
// player using the currently established websocket connection.
func WriteJSON(ctx context.Context, lobby *game.Lobby, player *game.Player, object interface{}) error {
	span := trace.SpanFromContext(ctx)
	traceId := span.SpanContext().TraceID.String()
	spanId := span.SpanContext().SpanID.String()

	switch object.(type) {
	case game.GameEvent:
		event := object.(game.GameEvent)
		event.TraceID = traceId
		event.SpanID = spanId
		object = event
	case game.LineEvent:
		event := object.(game.LineEvent)
		event.TraceID = traceId
		event.SpanID = spanId
		object = event
	case game.FillEvent:
		event := object.(game.FillEvent)
		event.TraceID = traceId
		event.SpanID = spanId
		object = event
	default:
	}

	if state.PubSub && lobby.IsReferenceReplica() {
		var event state.PersistedEvent
		event.LobbyId = lobby.LobbyID
		event.PlayerId = player.ID
		event.Data, _ = json.Marshal(object)

		realData, err := json.Marshal(event)
		if err == nil {
			state.PublishRedis(lobby.LobbyID+"-out", realData)

			//channelOut <- realData
		} else {
			log.Printf("error while marshalling writejson %s", err)
		}

		return nil
	} else {
		return sendJSONtoSocket(player, object)
	}
}

func pubSubOut() {
	for {
		var event state.PersistedEvent
		data := <-channelOut

		err := json.Unmarshal(data, &event)
		if err != nil {
			log.Fatalf("pubsubOut: unable to unmarshal %s", err)
		}
		var gameEvent game.GameEvent
		err = json.Unmarshal(event.Data, &gameEvent)
		if err != nil {
			log.Fatalf("pubsubOut: unable to unmarshal event %s", err)
		}

		var player *game.Player
		lobby := state.GetLobby(event.LobbyId)
		for _, p := range lobby.GetPlayers() {
			if p.ID == event.PlayerId {
				player = p
			}
		}
		if player == nil {
			// don't send event: player is probably on another server
			log.Printf("pubsubOut: player %s not found", event.PlayerId)

		} else {
			// send event only if player found
			//HandleEvent(lobby, player, data)
			sendJSONtoSocket(player, gameEvent)
		}

	}
}

func sendJSONtoSocket(player *game.Player, object interface{}) error {
	player.GetWebsocketMutex().Lock()
	defer player.GetWebsocketMutex().Unlock()

	socket := player.GetWebsocket()
	if socket == nil {
		return nil
	}
	if socket == nil || !player.Connected {
		return errors.New("player not connected")
	}

	return socket.WriteJSON(object)
}
