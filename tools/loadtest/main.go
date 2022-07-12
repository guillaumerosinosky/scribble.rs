package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/gorilla/websocket"
	"github.com/guillaumerosinosky/scribble.rs/game"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp"
	"go.opentelemetry.io/otel/exporters/otlp/otlpgrpc"
	"go.opentelemetry.io/otel/exporters/stdout"
	"go.opentelemetry.io/otel/metric/global"
	"go.opentelemetry.io/otel/propagation"
	controller "go.opentelemetry.io/otel/sdk/metric/controller/basic"
	processor "go.opentelemetry.io/otel/sdk/metric/processor/basic"
	"go.opentelemetry.io/otel/sdk/metric/selector/simple"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/semconv"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"
)

type LobbyData struct {
	LobbyID                string `json:"lobbyId"`
	DrawingBoardBaseWidth  int    `json:"drawingBoardBaseWidth"`
	DrawingBoardBaseHeight int    `json:"drawingBoardBaseHeight"`
}

func handleErr(err error, message string) {
	if err != nil {
		log.Fatalf("%s: %v", message, err)
	}
}

func initProvider(collectorEndpoint string, serviceName string) func() {
	ctx := context.Background()

	driver := otlpgrpc.NewDriver(
		otlpgrpc.WithInsecure(),
		otlpgrpc.WithEndpoint(collectorEndpoint),
		otlpgrpc.WithDialOption(grpc.WithBlock()), // useful for testing
	)
	exp, err := otlp.NewExporter(ctx, driver)
	handleErr(err, "failed to create exporter")

	res, err := resource.New(ctx,
		resource.WithAttributes(
			// the service name used to display traces in backends
			semconv.ServiceNameKey.String(serviceName),
		),
	)
	handleErr(err, "failed to create resource")

	bsp := sdktrace.NewBatchSpanProcessor(exp)
	tracerProvider := sdktrace.NewTracerProvider(
		sdktrace.WithConfig(sdktrace.Config{DefaultSampler: sdktrace.AlwaysSample()}),
		sdktrace.WithResource(res),
		sdktrace.WithSpanProcessor(bsp),
	)

	cont := controller.New(
		processor.New(
			simple.NewWithExactDistribution(),
			exp,
		),
		controller.WithPusher(exp),
		controller.WithCollectPeriod(2*time.Second),
	)

	// set global propagator to tracecontext (the default is no-op).
	otel.SetTextMapPropagator(propagation.TraceContext{})
	otel.SetTracerProvider(tracerProvider)
	global.SetMeterProvider(cont.MeterProvider())
	handleErr(cont.Start(context.Background()), "failed to start controller")

	return func() {
		// Shutdown will flush any remaining spans.
		handleErr(tracerProvider.Shutdown(ctx), "failed to shutdown TracerProvider")

		// Push any last metric events to the exporter.
		handleErr(cont.Stop(context.Background()), "failed to stop controller")
	}
}

func getMessage(ctx context.Context, socket *websocket.Conn) (*game.GameEvent, error) {
	messageType, data, err := socket.ReadMessage()
	if err != nil {
		log.Printf("Error on socket read: %s", err.Error())
		return nil, err
	}
	log.Printf("<- %d %s ", messageType, data)
	if messageType == websocket.TextMessage {
		received := &game.GameEvent{}
		err = json.Unmarshal(data, received)
		tracer := otel.Tracer("loadtest")
		var span trace.Span

		log.Printf("message %s: %s", received.Type, received.Data)
		if received.TraceID != "" {
			traceId, _ := trace.TraceIDFromHex(received.TraceID)
			spanId, _ := trace.SpanIDFromHex(received.SpanID)
			sc := trace.SpanContext{
				TraceID:    traceId,
				SpanID:     spanId,
				TraceFlags: 0x0,
			}
			ctx, span = tracer.Start(trace.ContextWithRemoteSpanContext(ctx, sc), "handleEvent")
		} else {
			ctx, span = tracer.Start(context.Background(), "handleEvent")
		}
		span.End()

		return received, err
	}
	log.Printf("Message not managed")
	return nil, nil
}

func waitForCommand(ctx context.Context, socket *websocket.Conn, commands []string) *game.GameEvent {
	log.Printf("wait for %s", commands)
	//var result *game.GameEvent

	for start := time.Now(); time.Since(start) < 10*time.Second; {
		result, err := getMessage(ctx, socket)
		if err != nil {
			log.Fatalf("Error on getMessage %s", err)
		}

		for _, command := range commands {
			if result.Type == command {
				return result
			}
		}
	}
	return nil
}

func sendCommand(ctx context.Context, socket *websocket.Conn, command string, data interface{}) error {
	log.Printf("-> %s %s ", command, data)
	tracer := otel.Tracer("loadtest")
	_, span := tracer.Start(ctx, command)
	defer span.End()
	traceId := span.SpanContext().TraceID.String()
	spanId := span.SpanContext().SpanID.String()
	message := map[string]interface{}{
		"type":    command,
		"data":    data,
		"traceId": traceId,
		"spanId":  spanId,
	}
	return socket.WriteJSON(message)
}

func keepAlive(ctx context.Context, socket *websocket.Conn, duration int) {
	for {
		sendCommand(ctx, socket, "keep-alive", "")

		log.Printf("send keep-alive")
		time.Sleep(1 * time.Second)
		duration = duration - 1
		if duration == 0 {
			break
		}
	}
}

func draw(ctx context.Context, socket *websocket.Conn) {
	sendCommand(ctx, socket, "choose-word", 0)
	log.Printf("choose word")

	var events []game.GameEvent
	jsonFile, err := os.Open("stickman.json")
	byteValue, _ := ioutil.ReadAll(jsonFile)
	if err != nil {
		log.Fatalf("drawing file not found %s", err)
	}
	json.Unmarshal(byteValue, &events)

	for _, event := range events {
		json.Marshal(event)
		sendCommand(ctx, socket, event.Type, event)
		socket.WriteJSON(event)
	}
}

func round(ctx context.Context, socket *websocket.Conn) {

	eventStartRound := waitForCommand(ctx, socket, []string{"your-turn", "update-wordhint"})
	if eventStartRound == nil {
		log.Printf("No answer")
		return
	}
	if eventStartRound.Type == "your-turn" { // turn to draw
		// Choose word
		sendCommand(ctx, socket, "choose-word", 0)
		// Sleep a bit
		time.Sleep(2 * time.Second)

		// Draw
		draw(ctx, socket)

	} else { // turn to guess (update-word-hint)
		// Sleep a bit
		time.Sleep(2 * time.Second)

		for i := 1; i < 5; i++ {
			sendCommand(ctx, socket, "message", fmt.Sprintf("不知道-%d", i))
		}

	}

	// wait for end of turn
	waitForCommand(ctx, socket, []string{"next-turn"})
}

func owner(ctx context.Context, serverHost string, lobbyID string, roundQuantity int) {
	//Create new lobby. This will return the ID of the newly created lobby.
	r, _ := http.PostForm(fmt.Sprintf("http://%s/v1/lobby", serverHost), url.Values{
		"username":             {"Marcel"},
		"language":             {"english"},
		"max_players":          {"24"},
		"drawing_time":         {"30"},
		"rounds":               {"5"},
		"custom_words_chance":  {"50"},
		"enable_votekick":      {"true"},
		"clients_per_ip_limit": {"10"},
		"public":               {"true"},
		"custom_lobby_id":      {lobbyID},
	})

	rawData, _ := ioutil.ReadAll(r.Body)
	lobbyData := &LobbyData{}
	json.Unmarshal(rawData, lobbyData)

	//The usersession gets stored in a cookie
	userSession := r.Cookies()[0].Value
	log.Println("Created lobby: " + lobbyData.LobbyID)
	log.Println("Usersession: " + userSession)

	//Connecting the socket by setting the Usersession and dialing.
	u := &url.URL{Scheme: "ws", Host: serverHost, Path: "/v1/ws", RawQuery: "lobby_id=" + lobbyData.LobbyID}
	header := make(http.Header)
	header["Usersession"] = []string{userSession}
	socket, _, _ := websocket.DefaultDialer.Dial(u.String(), header)
	defer socket.Close()

	//Do stuff with connection ...
	sendCommand(ctx, socket, "name-change", "owner")
	sendCommand(ctx, socket, "start", "")

	// TODO: keepAlive hangs, need to fix it
	//keepAliveContext, _ := context.WithCancel(context.Background())
	//go keepAlive(keepAliveContext, socket, 10)

	waitForCommand(ctx, socket, []string{"ready"})
	waitForCommand(ctx, socket, []string{"update-players"})
	waitForCommand(ctx, socket, []string{"next-turn"})

	for i := 1; i < roundQuantity; i++ {
		log.Printf("Round %d", i)
		round(ctx, socket)
	}
}

func player(ctx context.Context, serverHost string, lobbyId string, roundQuantity int) {
	r, _ := http.Get(fmt.Sprintf("http://%s/ssrEnterLobby?lobby_id=%s", serverHost, lobbyId))

	userSession := r.Cookies()[0].Value

	//Connecting the socket by setting the Usersession and dialing.
	u := &url.URL{Scheme: "ws", Host: "localhost:8080", Path: "/v1/ws", RawQuery: "lobby_id=" + lobbyId}
	header := make(http.Header)
	header["Usersession"] = []string{userSession}
	socket, _, _ := websocket.DefaultDialer.Dial(u.String(), header)
	defer socket.Close()
	sendCommand(ctx, socket, "name-change", "player")
	for i := 1; i < roundQuantity; i++ {
		log.Printf("Round %d", i)
		round(ctx, socket)
	}
}

func main() {

	serviceName, serviceNameSet := os.LookupEnv("SERVICE_NAME")
	if !serviceNameSet {
		serviceName = "LoadTest"
	}

	telemetryActivated, _ := os.LookupEnv("OTEL")

	telemetryServer, telemetryServerAvailable := os.LookupEnv("OTEL_HOST")
	ctx := context.Background()
	if telemetryActivated == "true" {
		if telemetryServerAvailable {
			initProvider(telemetryServer, serviceName)
		} else {
			exporter, err := stdout.NewExporter(
				stdout.WithPrettyPrint(),
			)
			if err != nil {
				log.Fatalf("failed to initialize stdout export pipeline: %v", err)
			}

			bsp := sdktrace.NewBatchSpanProcessor(exporter)
			tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(bsp))
			otel.SetTracerProvider(tp)

			tracer := otel.Tracer("loadtest")
			func(ctx context.Context) {
				_, span := tracer.Start(ctx, "operation")
				log.Printf("%s", span)

				defer span.End()
			}(ctx)

			// Handle this error in a sensible manner where possible
			defer func() { _ = tp.Shutdown(ctx) }()
		}
	}

	serverHost, serverHostAvailable := os.LookupEnv("SERVER_HOST")
	if !serverHostAvailable {
		serverHost = "localhost:8080"
	}

	userType, userTypeAvailable := os.LookupEnv("USER_TYPE")
	if !userTypeAvailable {
		userType = "owner"
	}

	lobbyID, lobbyIdAvailable := os.LookupEnv("LOBBY_ID")
	if !lobbyIdAvailable {
		lobbyID = "test"
	}

	if userType == "owner" {
		owner(ctx, serverHost, lobbyID, 3)
	} else {
		player(ctx, serverHost, lobbyID, 3)
	}
}
