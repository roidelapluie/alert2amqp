package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"runtime"
	"sync"
	"time"

	log "github.com/Sirupsen/logrus"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"pack.ag/amqp"

	kingpin "gopkg.in/alecthomas/kingpin.v2"
)

var (
	username string
	password string
	mapLock  = &sync.RWMutex{}
	amqpURL  = kingpin.Flag("amqp", "AMQP URL").Default("amqp://127.0.0.1:5672").String()
	ip       = kingpin.Flag("listen", "IP/Port to listen to").Default(":2778").String()
	level    = kingpin.Flag("log-level", "log level").Default("info").String()
	link     = kingpin.Flag("link", "Link").Default("alerts").String()
	client   *amqp.Client
	session  *amqp.Session
	sender   *amqp.Sender
	ctx      context.Context
	upInfo   = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Namespace: "amqp",
			Name:      "up",
			Help:      "Status of the AMQP connection",
		},
	)
)

func main() {
	kingpin.Parse()
	l, err := log.ParseLevel(*level)
	if err != nil {
		log.Fatal(err)
	}
	log.SetLevel(l)
	username = os.Getenv("AMQP_USER")
	if username == "" {
		log.Fatal("AMQP_USER can't be empty")
	}
	password = os.Getenv("AMQP_PASSWORD")
	if password == "" {
		log.Fatal("AMQP_PASSWORD can't be empty")
	}

	inFlightGauge := prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "alert",
		Help: "A gauge of requests currently being served by the wrapped handler.",
	})

	duration := prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "amqp_handler_request_duration_seconds",
			Help:    "A histogram of latencies for requests.",
			Buckets: []float64{.25, .5, 1, 2.5, 5, 10},
		},
		[]string{"method"},
	)

	upInfo.Set(0)
	prometheus.MustRegister(upInfo)

	buildInfo := prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "alert2amqp",
			Name:      "build_info",
			Help: fmt.Sprintf(
				"A metric with a constant '1' value labeled by version, revision, branch, and goversion from which %s was built.",
				"alert2amqp",
			),
		},
		[]string{"version", "goversion"},
	)
	buildInfo.WithLabelValues(version, runtime.Version()).Set(1)
	prometheus.MustRegister(buildInfo)

	counter := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "amqp_handler_requests_total",
			Help: "A counter for requests to the wrapped handler.",
		},
		[]string{"code", "method"},
	)

	amqpChain := promhttp.InstrumentHandlerInFlight(inFlightGauge,
		promhttp.InstrumentHandlerDuration(duration,
			promhttp.InstrumentHandlerCounter(counter,
				&amqpHandler{},
			),
		),
	)
	prometheus.MustRegister(inFlightGauge, counter, duration)

	http.Handle("/metrics", promhttp.Handler())
	http.Handle("/", amqpChain)

	log.Infof("Listering on %s", *ip)

	go connectAMQP()

	log.Fatal(http.ListenAndServe(*ip, nil))
}

// recoverAMQP is called when the AMQP connection is not working
func recoverAMQP() {
	upInfo.Set(0)
	closeCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if sender != nil {
		sender.Close(closeCtx)
	}
	if session != nil {
		session.Close(closeCtx)
	}
	if client != nil {
		client.Close()
	}
	connectAMQP()
}

func connectAMQP() {
	var err error
	client, err = amqp.Dial(*amqpURL,
		amqp.ConnSASLPlain(username, password),
	)
	if err != nil {
		log.Error("Dialing AMQP server:", err)
		return
	}

	session, err = client.NewSession()
	if err != nil {
		log.Error("Creating AMQP session:", err)
		return
	}

	sender, err = session.NewSender(
		amqp.LinkTargetAddress(*link),
	)
	if err != nil {
		log.Error("Creating Sender: ", err)
		return
	}
	upInfo.Set(1)
}
