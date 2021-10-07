package state

import (
	"encoding/json"
	"fmt"
	"log"
	"os"

	"github.com/gomodule/redigo/redis"
	"github.com/guillaumerosinosky/scribble.rs/game"
)

var (
	Persistence     bool
	DatabaseHost    string
	PersistenceMode string
	PubSub          bool
)

type PersistedEvent struct {
	LobbyId  string
	PlayerId string
	Data     []byte
}

func PublishRedis(channel string, message []byte) {
	c, err := redis.Dial("tcp", DatabaseHost+":6379")
	if err != nil {
		log.Println(err)
		os.Exit(1)
	}
	c.Do("PUBLISH", channel, message)
}

func SubscribeRedis(pChannel string, channel chan []byte) {
	c, err := redis.Dial("tcp", DatabaseHost+":6379")
	if err != nil {
		log.Println(err)
		os.Exit(1)
	}
	psc := redis.PubSubConn{Conn: c}
	psc.Subscribe(pChannel)
	for {
		switch v := psc.Receive().(type) {
		case redis.Message:
			fmt.Printf("%s: message: %s\n", v.Channel, v.Data)
			channel <- v.Data

		case redis.Subscription:
			fmt.Printf("%s: %s %d\n", v.Channel, v.Kind, v.Count)
		case error:
			fmt.Println(v)
		}
	}
}

func OnSubscribeMessageRedis() {

}

func nPool() *redis.Pool {
	return &redis.Pool{
		MaxIdle:   50,
		MaxActive: 10000,
		Dial: func() (redis.Conn, error) {
			conn, err := redis.Dial("tcp", DatabaseHost+":6379")
			if err != nil {
				log.Printf("ERROR: fail initializing the redis pool: %s", err.Error())
				os.Exit(1)
			}
			return conn, err
		},
	}
}

func DeleteLobby(id string) {
	rPool := nPool()
	conn := rPool.Get()
	defer conn.Close()
	_, err := conn.Do("DEL", id)
	if err != nil {
		log.Fatalf("Error while deleting lobby %s : %s", id, err)
	}
	//log.Printf("Result: %s", values.(string))
}

func SaveLobby(lobby *game.Lobby) {
	rPool := nPool()
	conn := rPool.Get()
	defer conn.Close()
	_, err := conn.Do("SET", lobby.LobbyID, LobbyToJson(lobby))
	if err != nil {
		log.Fatalf("Error while saving lobby %s : %s", lobby.LobbyID, err)
	}
	//log.Printf("Result: %s", values.(string))

}

func NoSaveLobby(lobby *game.Lobby) {
	// default behaviour
}

func AddLobbyEvent(lobby *game.Lobby) {

}

func LoadLobby(key string) *game.Lobby {
	rPool := nPool()
	conn := rPool.Get()
	defer conn.Close()
	value, err := redis.String(conn.Do("GET", key))
	if err != nil {
		log.Fatalf("Error while saving lobby %s : %s", key, err)
	}
	log.Printf("Load lobby: %s", value)
	return JsonToLobby(value)
}

func LoadLobbyList() []string {
	if Persistence {
		rPool := nPool()
		conn := rPool.Get()
		values, err := redis.Strings(conn.Do("KEYS", "*"))
		if err != nil {
			log.Fatalf("Error while gettings keys %s", err)
		}
		return values
	} else {
		return []string{}
	}
}

func LobbyToJson(lobby *game.Lobby) string {
	m := game.MarshallLobby(lobby)
	result, err := json.Marshal(m)
	if err != nil {
		log.Fatalf("LobbyToJson: %s", err)
	}
	//log.Printf("%s", string(result))
	return string(result)
}

func JsonToLobby(value string) *game.Lobby {
	var l game.LobbyEntity
	err := json.Unmarshal([]byte(value), &l)
	if err != nil {
		log.Fatalf("JsonToLobby: %s", err)
	}
	return game.UnmarshallLobby(l)
}

/*
func clearFigure(lobby *game.Lobby) {
	rPool := nPool()
	conn := rPool.Get()
	values, err := redis.Strings(conn.Do("KEYS"))
}

func appendLine(lobby *game.Lobby) {

}
*/
