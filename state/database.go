package state

import (
	"encoding/json"
	"log"
	"os"

	"github.com/gomodule/redigo/redis"
	"github.com/guillaumerosinosky/scribble.rs/game"
)

var (
	Persistence  bool
	DatabaseHost string
)

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
	values, err := conn.Do("SET", lobby.LobbyID, LobbyToJson(lobby))
	if err != nil {
		log.Fatalf("Error while saving lobby %s : %s", lobby.LobbyID, err)
	}
	log.Printf("Result: %s", values.(string))

}

func LoadLobby(key string) *game.Lobby {
	rPool := nPool()
	conn := rPool.Get()
	defer conn.Close()
	value, err := redis.String(conn.Do("GET", key))
	if err != nil {
		log.Fatalf("Error while saving lobby %s : %s", key, err)
	}
	log.Printf("Result: %s", value)
	return JsonToLobby(value)
}

func LoadLobbyList() []string {
	rPool := nPool()
	conn := rPool.Get()
	values, err := redis.Strings(conn.Do("KEYS", "*"))
	if err != nil {
		log.Fatalf("Error while gettings keys %s", err)
	}
	return values
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
