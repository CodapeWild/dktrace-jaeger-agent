# Jaeger APM Tracing Data Simulator For [Datakit](https://github.com/GuanceCloud/datakit)

**Notice:** THIS PROJECT IS STILL IN PROGRESS

This tool uses standard Jaeger APM golang-lib to simulate APM data and send to Datakit agent for correctness and pressure test purposes.

- build with [standard Jaeger APM Golang lib](https://github.com/jaegertracing/jaeger-client-go)
- customized Span data
- configurable multi-thread pressure test

## Config

**Config structure in `config.json`**

```json
{
  "dk_agent": "127.0.0.1:9529",
  "protocol": "http",
  "sender": {
    "threads": 1,
    "send_count": 1
  },
  "service": "dktrace-jaeger-agent",
  "dump_size": 1024,
  "random_dump": true,
  "trace": []
}
```

- `dk_agent`: Datakit host address
- `protocol`: network transport protocol, HTTP or UDP
- `sender.threads`: how many threads will start to send `trace` simultaneously
- `sender.send_count`: how many times `trace` will be send in one `thread`
- `service`: service name
- `dump_size`: the data size in kb used to fillup the trace, 0: no extra data
- `random_dump`: whether to fillup the span with random size extra data
- `trace`: represents a Trace consists of Spans

## Span Structure

**Span structure in `config.json`**

`trace`\[span...\]

```json
{
  "resource": "/get/user/name",
  "operation": "user.getUserName",
  "span_type": "",
  "duration": 1000,
  "error": "access deny, status code 100010",
  "tags": [
    {
      "key": "",
      "value": ""
    }
  ],
  "children": []
}
```

**Note:** Spans list in `trace` or `children` will generate concurrency Spans, nested in `trace` or `children` will show up as calling chain.

- `resource`: resource name
- `operation`: operation name
- `span_type`: Span type [app cache custom db web]
- `duration`: how long an operation process will last
- `error`: error string
- `tags`: Span meta data
- `children`: child Spans represent a subsequent function calling from current `operation`
