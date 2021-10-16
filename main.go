package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/gofrs/uuid"
	"github.com/guillaumerosinosky/scribble.rs/api"
	"github.com/guillaumerosinosky/scribble.rs/frontend"
	"github.com/guillaumerosinosky/scribble.rs/game"
	"github.com/guillaumerosinosky/scribble.rs/state"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/stdout"

	"go.opentelemetry.io/otel/exporters/otlp"
	"go.opentelemetry.io/otel/exporters/otlp/otlpgrpc"
	"go.opentelemetry.io/otel/metric/global"
	"go.opentelemetry.io/otel/propagation"
	controller "go.opentelemetry.io/otel/sdk/metric/controller/basic"
	processor "go.opentelemetry.io/otel/sdk/metric/processor/basic"
	"go.opentelemetry.io/otel/sdk/metric/selector/simple"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/semconv"
	"google.golang.org/grpc"
)

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

func main() {
	log.Printf("Starting Scribblers")
	portHTTPFlag := flag.Int("portHTTP", -1, "defines the port to be used for http mode")
	flag.Parse()

	var portHTTP int
	if *portHTTPFlag != -1 {
		portHTTP = *portHTTPFlag
		log.Printf("Listening on port %d sourced from portHTTP flag.\n", portHTTP)
	} else {
		//Support for heroku, as heroku expects applications to use a specific port.
		envPort, portVarAvailable := os.LookupEnv("PORT")
		if portVarAvailable {
			log.Printf("'PORT' environment variable found: '%s'\n", envPort)
			parsed, parseError := strconv.ParseInt(envPort, 10, 32)
			if parseError == nil {
				portHTTP = int(parsed)
				log.Printf("Listening on port %d sourced from 'PORT' environment variable\n", portHTTP)
			} else {
				log.Printf("Error parsing 'PORT' variable: %s\n", parseError)
				log.Println("Falling back to default port.")
			}
		}
	}

	if portHTTP == 0 {
		portHTTP = 8080
		log.Printf("Listening on default port %d\n", portHTTP)
	}

	databaseServer, databaseAvailable := os.LookupEnv("DB_HOST")
	if databaseAvailable {
		state.DatabaseHost = databaseServer
		state.Persistence = true
		log.Printf("Persistence enabled on %s redis Server\n", state.DatabaseHost)

	} else {
		state.Persistence = false
	}

	pubSub, pubSubAvailable := os.LookupEnv("PUBSUB")
	if pubSubAvailable && pubSub == "true" {
		state.PubSub = true
		//go state.SubscribeRedis()
	} else {
		state.PubSub = false
	}

	telemetryActivated, _ := os.LookupEnv("OTEL")
	serviceName, serviceNameSet := os.LookupEnv("SERVICE_NAME")
	if !serviceNameSet {
		serviceName = "Lobby"
	}
	persistenceModeSet := false
	persistenceMode, persistenceModeSet := os.LookupEnv("PERSISTENCE_MODE")
	if !persistenceModeSet {
		persistenceMode = "NONE"
	}
	state.PersistenceMode = persistenceMode

	telemetryServer, telemetryServerAvailable := os.LookupEnv("OTEL_HOST")

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
			ctx := context.Background()
			bsp := sdktrace.NewBatchSpanProcessor(exporter)
			tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(bsp))
			otel.SetTracerProvider(tp)

			tracer := otel.Tracer("lobby")
			func(ctx context.Context) {
				_, span := tracer.Start(ctx, "operation")
				log.Printf("%s", span)

				defer span.End()
			}(ctx)

			// Handle this error in a sensible manner where possible
			defer func() { _ = tp.Shutdown(ctx) }()
		}
	}

	//Setting the seed in order for the petnames to be random.
	rand.Seed(time.Now().UnixNano())

	game.ReplicaID = uuid.Must(uuid.NewV4()).String()

	log.Println("Started replica {}.", game.ReplicaID)

	api.SetupRoutes()
	frontend.SetupRoutes()

	http.ListenAndServe(fmt.Sprintf(":%d", portHTTP), nil)
}
