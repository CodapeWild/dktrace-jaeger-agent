package main

import (
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/CodapeWild/devtools/idflaker"
	"github.com/opentracing/opentracing-go"
	jaeger "github.com/uber/jaeger-client-go"
	"github.com/uber/jaeger-client-go/transport"
	"github.com/uber/jaeger-client-go/utils"
)

var (
	cfg          *config
	idflk        *idflaker.IDFlaker
	globalCloser = make(chan struct{})
	agentAddress = "127.0.0.1:"
	path         = "/apis/traces"
)

type sender struct {
	Threads      int `json:"threads"`
	SendCount    int `json:"send_count"`
	SendInterval int `json:"send_interval"`
}

type config struct {
	DkAgent    string  `json:"dk_agent"`
	Protocol   string  `json:"protocol"`
	Sender     *sender `json:"sender"`
	Service    string  `json:"service"`
	DumpSize   int     `json:"dump_size"`
	RandomDump bool    `json:"random_dump"`
	Trace      []*span `json:"trace"`
}

type tag struct {
	Key   string      `json:"key"`
	Value interface{} `json:"value"`
}

type span struct {
	Resource  string  `json:"resource"`
	Operation string  `json:"operation"`
	SpanType  string  `json:"span_type"`
	Duration  int64   `json:"duration"`
	Error     string  `json:"error"`
	Tags      []tag   `json:"tags"`
	Children  []*span `json:"children"`
	dumpSize  int64
}

func main() {
	var trans jaeger.Transport
	switch cfg.Protocol {
	case "http":
		urlStr := fmt.Sprintf("http://%s%s", cfg.DkAgent, path)
		log.Printf("Jaeger Agent Address: %s", urlStr)
		trans = transport.NewHTTPTransport(urlStr)
		// TODO: start HTTP agent to accept message
	case "udp":
		var err error
		if trans, err = jaeger.NewUDPTransport(agentAddress, utils.UDPPacketMaxLength); err != nil {
			log.Fatalln(err.Error())
		}
		// TODO: start UDP agent to accept packet
	default:
		log.Fatalln(fmt.Printf("unsupported scheme: %s\n", cfg.Protocol))
	}

	reporter := jaeger.NewRemoteReporter(trans)
	defer reporter.Close()

	tracer, closer := jaeger.NewTracer(cfg.Service, jaeger.NewConstSampler(true), reporter)
	defer closer.Close()
	opentracing.SetGlobalTracer(tracer)

	span := tracer.StartSpan("root")
	defer span.Finish()

	foo1(span.Context())
}

func foo1(ctx opentracing.SpanContext) {
	span := opentracing.GlobalTracer().StartSpan("foo1", opentracing.ChildOf(ctx))
	defer span.Finish()

	foo2(span.Context())
}

func foo2(ctx opentracing.SpanContext) {
	span := opentracing.GlobalTracer().StartSpan("foo2", opentracing.ChildOf(ctx))
	defer span.Finish()
}

func init() {
	log.SetOutput(os.Stdout)
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	data, err := os.ReadFile("./config.json")
	if err != nil {
		log.Fatalln(err.Error())
	}

	cfg = &config{}
	if err = json.Unmarshal(data, cfg); err != nil {
		log.Fatalln(err.Error())
	}
	cfg.Protocol = strings.ToLower(cfg.Protocol)

	if idflk, err = idflaker.NewIdFlaker(66); err != nil {
		log.Fatalln(err.Error())
	}

	rand.Seed(time.Now().UnixNano())
	agentAddress += strconv.Itoa(30000 + rand.Intn(10000))
}
