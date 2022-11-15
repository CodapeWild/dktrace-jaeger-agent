package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/uber/jaeger-client-go/thrift"
	"github.com/uber/jaeger-client-go/thrift-gen/agent"
	"github.com/uber/jaeger-client-go/thrift-gen/jaeger"
	"github.com/uber/jaeger-client-go/utils"
)

func startHTTPAgent() {
	log.Printf("### start Jaeger APM agent(HTTP) %s\n", agentAddress)

	svr := getTimeoutServer(agentAddress, http.HandlerFunc(handleJaegerTraceData))
	go func() {
		if err := svr.ListenAndServe(); err != nil {
			log.Fatalln(err.Error())
		}
	}()
}

func getTimeoutServer(address string, handler http.Handler) *http.Server {
	return &http.Server{
		Addr:              address,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Millisecond,
		ReadTimeout:       time.Second,
		WriteTimeout:      3 * time.Second,
		IdleTimeout:       10 * time.Second,
	}
}

func handleJaegerTraceData(resp http.ResponseWriter, req *http.Request) {
	log.Println("### Jaeger APM original headers:")
	for k, v := range req.Header {
		log.Printf("%s: %v\n", k, v)
	}

	resp.WriteHeader(http.StatusOK)

	buf, err := io.ReadAll(req.Body)
	if err != nil {
		log.Fatalln(err.Error())
	}

	sendTraceOverHTTP(cfg.DkAgent, buf, fmt.Sprintf("http://%s%s", cfg.DkAgent, path), req.Header)
}

func sendTraceOverHTTP(dkAgent string, buf []byte, endpoint string, header http.Header) {
	batch := decodeBinaryProtocol(buf)

	wg := &sync.WaitGroup{}
	wg.Add(cfg.Sender.Threads)
	for i := 0; i < cfg.Sender.Threads; i++ {
		dupi := shallowCopyBatch(batch)

		go func(batch *jaeger.Batch) {
			for j := 0; j < cfg.Sender.SendCount; j++ {
				modifyTraceID(batch)
				buf := encodeBinaryProtocol(dupi)

				req, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewBuffer(buf))
				if err != nil {
					log.Fatalln(err.Error())
				}
				req.Header = header

				resp, err := http.DefaultClient.Do(req)
				if err != nil {
					log.Println(err.Error())
					continue
				}
				log.Println(resp.Status)
				resp.Body.Close()
			}
		}(dupi)
	}
	wg.Wait()

	close(globalCloser)
}

func decodeBinaryProtocol(buf []byte) *jaeger.Batch {
	tmbuf := thrift.NewTMemoryBuffer()
	_, err := tmbuf.ReadFrom(bytes.NewBuffer(buf))
	if err != nil {
		log.Fatalln(err.Error())
	}

	var (
		transport = thrift.NewTBinaryProtocolConf(tmbuf, &thrift.TConfiguration{})
		batch     = &jaeger.Batch{}
	)
	if err = batch.Read(context.Background(), transport); err != nil {
		log.Fatalln(err.Error())
	}

	return batch
}

func encodeBinaryProtocol(batch *jaeger.Batch) []byte {
	var (
		tmbuf     = thrift.NewTMemoryBuffer()
		transport = thrift.NewTBinaryProtocolConf(tmbuf, &thrift.TConfiguration{})
	)
	if err := batch.Write(context.Background(), transport); err != nil {
		log.Fatalln(err.Error())
	}

	return tmbuf.Bytes()
}

func startUDPAgent() {
	udpAddr, err := net.ResolveUDPAddr("udp", agentAddress)
	if err != nil {
		log.Fatalln(err.Error())
	}

	conn, err := net.ListenUDP("udp", udpAddr)
	if err != nil {
		log.Fatalln(err.Error())
	}

	buf := make([]byte, utils.UDPPacketMaxLength)
	go func() {
		for {
			select {
			case <-globalCloser:
				conn.Close()

				return
			default:
			}

			conn.SetDeadline(time.Now().Add(10 * time.Second))
			n, err := conn.Read(buf)
			if err != nil {
				log.Println(err.Error())
				continue
			}

			sendTraceOverUDP(cfg.DkAgent, buf[:n])
		}
	}()
}

func sendTraceOverUDP(dkAgent string, buf []byte) {
	conn, err := net.Dial("udp", dkAgent)
	if err != nil {
		log.Fatalln(err.Error())
	}

	batchArgs := decodeCompactBinaryProtocol(buf)

	wg := sync.WaitGroup{}
	wg.Add(cfg.Sender.Threads)
	for i := 0; i < cfg.Sender.Threads; i++ {
		dupi := shallowCopyBatch(batchArgs.Batch)

		go func(batch *jaeger.Batch) {
			defer wg.Done()

			for j := 0; j < cfg.Sender.SendCount; j++ {
				modifyTraceID(batch)
				buf := encodeCompactBinaryProtocol(&agent.AgentEmitBatchArgs{Batch: batch})
				if _, err := conn.Write(buf); err != nil {
					log.Fatalln(err.Error())
				}
			}
		}(dupi)
	}
}

func decodeCompactBinaryProtocol(buf []byte) *agent.AgentEmitBatchArgs {
	tmbuf := thrift.NewTMemoryBufferLen(len(buf))
	_, err := tmbuf.Write(buf)
	if err != nil {
		log.Fatalln(err.Error())
	}

	var (
		tprot = thrift.NewTCompactProtocolFactoryConf(&thrift.TConfiguration{}).GetProtocol(tmbuf)
		ctx   = context.Background()
	)
	if name, typeid, seqid, err := tprot.ReadMessageBegin(ctx); err != nil {
		log.Fatalln(err.Error())
	} else {
		log.Printf("### resolved Thrift Message name: %s, type: %d, seq_id: %d\n", name, typeid, seqid)
	}
	defer func() {
		if err := tprot.ReadMessageEnd(ctx); err != nil {
			log.Println(err.Error())
		}
	}()

	batch := &agent.AgentEmitBatchArgs{}
	if err = batch.Read(context.Background(), tprot); err != nil {
		log.Fatalln(err.Error())
	}

	return batch
}

func encodeCompactBinaryProtocol(batchArgs *agent.AgentEmitBatchArgs) []byte {
	var (
		tmbuf = thrift.NewTMemoryBuffer()
		tprot = thrift.NewTCompactProtocolFactoryConf(&thrift.TConfiguration{}).GetProtocol(tmbuf)
		ctx   = context.Background()
	)
	err := tprot.WriteMessageBegin(ctx, "emitBatch", thrift.CALL, 1)
	if err != nil {
		log.Fatalln(err.Error())
	}
	defer func() {
		if err = tprot.WriteMessageEnd(ctx); err != nil {
			log.Println(err.Error())
		}
	}()

	if err = batchArgs.Write(ctx, tprot); err != nil {
		log.Fatal(err.Error())
	}

	return tmbuf.Bytes()
}

func shallowCopyBatch(src *jaeger.Batch) *jaeger.Batch {
	dest := &jaeger.Batch{}
	*dest = *src
	dest.Spans = make([]*jaeger.Span, len(src.Spans))
	for i := range src.Spans {
		dest.Spans[i] = &jaeger.Span{
			TraceIdLow:    src.Spans[i].TraceIdLow,
			TraceIdHigh:   src.Spans[i].TraceIdHigh,
			SpanId:        src.Spans[i].SpanId,
			ParentSpanId:  src.Spans[i].ParentSpanId,
			OperationName: src.Spans[i].OperationName,
			References:    src.Spans[i].References,
			Flags:         src.Spans[i].Flags,
			StartTime:     src.Spans[i].StartTime,
			Duration:      src.Spans[i].Duration,
			Tags:          src.Spans[i].Tags,
			Logs:          src.Spans[i].Logs,
		}
	}

	return dest
}

func modifyTraceID(src *jaeger.Batch) {
	var (
		lid = idflk.NextInt64Id()
		hid = idflk.NextInt64Id()
	)
	for i := range src.Spans {
		if src.Spans[i].TraceIdLow != 0 {
			src.Spans[i].TraceIdLow = lid
		}
		if src.Spans[i].TraceIdHigh != 0 {
			src.Spans[i].TraceIdHigh = hid
		}
	}
}
