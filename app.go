package main

import (
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"os"
	"strconv"
	"time"

	"github.com/CodapeWild/devtools/idflaker"
	jconfig "github.com/uber/jaeger-client-go/config"
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
	jcfg := jconfig.Configuration{
		ServiceName: cfg.Service,
		Reporter: &jconfig.ReporterConfig{
			CollectorEndpoint: fmt.Sprintf("%s://%s", cfg.Protocol, agentAddress),
		},
	}
	closer, err := jcfg.InitGlobalTracer(cfg.Service)
	if err != nil {
		log.Fatalln(err.Error())
	}
	defer closer.Close()

}

func foo1() {}

func foo3() {}

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

	if idflk, err = idflaker.NewIdFlaker(66); err != nil {
		log.Fatalln(err.Error())
	}

	rand.Seed(time.Now().UnixNano())
	agentAddress += strconv.Itoa(30000 + rand.Intn(10000))
}
