package main

import (
	"encoding/hex"
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
	Resource  string        `json:"resource"`
	Operation string        `json:"operation"`
	SpanType  string        `json:"span_type"`
	Duration  time.Duration `json:"duration"`
	Error     string        `json:"error"`
	Tags      []tag         `json:"tags"`
	Children  []*span       `json:"children"`
	dumpSize  int64
}

func (sp *span) startSpanFromContext(ctx opentracing.SpanContext) opentracing.Span {
	if opentracing.GlobalTracer() == nil {
		log.Fatalln("global tracer not enabled")
	}

	var otsp opentracing.Span
	if ctx != nil {
		otsp = opentracing.StartSpan(sp.Operation, opentracing.ChildOf(ctx))
	} else {
		otsp = opentracing.StartSpan(sp.Operation)
	}

	otsp.SetTag("resource.name", sp.Resource)
	otsp.SetTag("span.type", sp.SpanType)
	for _, tag := range sp.Tags {
		otsp.SetTag(tag.Key, tag.Value)
	}

	if len(sp.Error) != 0 {
		otsp.SetTag("error", sp.Error)
	}

	if sp.dumpSize != 0 {
		buf := make([]byte, sp.dumpSize)
		rand.Read(buf)

		otsp.SetTag("_dump_data", hex.EncodeToString(buf))
	}

	total := int64(sp.Duration * time.Millisecond)
	d := rand.Int63n(total)
	time.Sleep(time.Duration(d))
	go func() {
		time.Sleep(time.Duration(total - d))
		otsp.Finish()
	}()

	return otsp
}

func main() {
	var trans jaeger.Transport
	switch cfg.Protocol {
	case "http":
		urlStr := fmt.Sprintf("http://%s%s", agentAddress, path)
		log.Printf("Jaeger HTTP Agent Address: %s", urlStr)
		trans = transport.NewHTTPTransport(urlStr)
		// TODO: start HTTP agent to accept message
	case "udp":
		var err error
		if trans, err = jaeger.NewUDPTransport(agentAddress, utils.UDPPacketMaxLength); err != nil {
			log.Fatalln(err.Error())
		}
		log.Printf("Jaeger UDP Agent Address: %s", cfg.DkAgent)
		startUDPAgent()
	default:
		log.Fatalln(fmt.Printf("unsupported scheme: %s\n", strings.ToUpper(cfg.Protocol)))
	}

	reporter := jaeger.NewRemoteReporter(trans)
	defer reporter.Close()

	tracer, closer := jaeger.NewTracer(cfg.Service, jaeger.NewConstSampler(true), reporter)
	defer closer.Close()
	opentracing.SetGlobalTracer(tracer)

	root, children := startRootSpan(cfg.Trace)
	orchestrator(root, children)
	time.Sleep(3 * time.Second)

	<-globalCloser
}

func countSpans(trace []*span, c int) int {
	c += len(trace)
	for i := range trace {
		if len(trace[i].Children) != 0 {
			c = countSpans(trace[i].Children, c)
		}
	}

	return c
}

func setPerDumpSize(trace []*span, fillup int64, isRandom bool) {
	for i := range trace {
		if isRandom {
			trace[i].dumpSize = rand.Int63n(fillup)
		} else {
			trace[i].dumpSize = fillup
		}
		if len(trace[i].Children) != 0 {
			setPerDumpSize(trace[i].Children, fillup, isRandom)
		}
	}
}

func startRootSpan(trace []*span) (root opentracing.Span, children []*span) {
	var sp *span
	if len(trace) == 1 {
		sp = trace[0]
		children = trace[0].Children
	} else {
		sp = &span{
			Operation: "startRootSpan",
			SpanType:  "web",
			Duration:  time.Duration(60 + rand.Intn(300)),
		}
		children = trace
	}
	root = sp.startSpanFromContext(nil)

	return
}

func orchestrator(otsp opentracing.Span, children []*span) {
	if len(children) == 1 {
		otsp = children[0].startSpanFromContext(otsp.Context())
		orchestrator(otsp, children[0].Children)
	} else {
		for i := range children {
			go func(otsp opentracing.Span, sp *span) {
				otsp = sp.startSpanFromContext(otsp.Context())
				if len(sp.Children) != 0 {
					orchestrator(otsp, sp.Children)
				}
			}(otsp, children[i])
		}
	}
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
	if len(cfg.Trace) == 0 {
		log.Fatalln("empty trace")
	}
	cfg.Protocol = strings.ToLower(cfg.Protocol)

	if idflk, err = idflaker.NewIdFlaker(66); err != nil {
		log.Fatalln(err.Error())
	}

	rand.Seed(time.Now().UnixNano())
	agentAddress += strconv.Itoa(30000 + rand.Intn(10000))
}
