package trace

import (
	"encoding/json"
	"flag"
	"io/ioutil"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

type JaegerResponse struct {
	Traces []Trace `json:"data"`
}

type Trace struct {
	TraceID string `json:"traceID"`
	Spans   []Span `json:"spans"`
}

type Span struct {
	TraceID       string `json:"traceID"`
	SpanID        string `json:"spanID"`
	OperationName string `json:"operationName"`
	Tags          []Tag  `json:"tags"`
}

type Tag struct {
	Key   string      `json:"key"`
	Value interface{} `json:"value"`
}

const expectedSpanName = "HelloWorldSpan"

var sample = flag.String("sample", "none", "The name of the sample app used to query traces from Jaeger")

func TestTraceGeneratedFromSample(t *testing.T) {
	trace := getTrace(t)

	assert.NotNil(t, trace.TraceID)
	assert.Equal(t, 1, len(trace.Spans))

	span := trace.Spans[0]
	assert.Equal(t, expectedSpanName, span.OperationName)
	assert.Contains(t, span.Tags, Tag{Key: "foo", Value: "bar"}, "Span does not contain tag 'foo:bar'")
}

func TestTraceGeneratedFromSampleApi(t *testing.T) {
	// Call the /helloworld endpoint on the sample API that will generate the hello world span
	response := invokeSampleApi(t)

	trace := getTraceWithRetry(t)

	assert.Equal(t, "Hello world!", response)
	assert.NotNil(t, trace.TraceID)

	// find the span generated by the API
	var span Span
	for _, s := range trace.Spans {
		if s.OperationName == expectedSpanName {
			span = s
		}
	}

	assert.NotNil(t, span)
	assert.Contains(t, span.Tags, Tag{Key: "foo", Value: "bar"}, "Span does not contain tag 'foo:bar'")
}

func getTrace(t *testing.T) *Trace {
	t.Logf("Going to call Jaeger to fetch trace for sample: %s", *sample)
	r, err := http.Get("http://localhost:16686/api/traces?service=" + *sample)
	if err != nil {
		t.Fatalf("Failed getting trace from Jaeger: %v", err)
	}

	t.Log("Received 200 response from Jaeger")

	defer r.Body.Close()
	var data JaegerResponse

	err = json.NewDecoder(r.Body).Decode(&data)
	if err != nil {
		t.Fatalf("Failed decoding json response from Jaeger: %v", err)
	}

	// useful for CI runs
	json, _ := json.MarshalIndent(data, "", "  ")
	t.Logf("Data received from Jaeger: \n%s\n", json)

	if len(data.Traces) == 0 {
		return nil
	}

	return &data.Traces[0]
}

func getTraceWithRetry(t *testing.T) *Trace {
	backoffSchedule := []time.Duration{
		1 * time.Second,
		3 * time.Second,
		10 * time.Second,
	}

	var trace *Trace

	// do some retries until we Jaeger has it
	for _, backoff := range backoffSchedule {
		trace = getTrace(t)

		if trace != nil {
			break
		}

		t.Logf("Trace not found yet, retrying in %v\n", backoff)
		time.Sleep(backoff)
	}

	// All retries failed
	if trace == nil {
		t.Fatalf("Failed getting trace from Jaeger")
	}

	return trace
}

func invokeSampleApi(t *testing.T) string {
	t.Logf("Going to call the sample API to generate trace for sample: %s", *sample)
	r, err := http.Get("http://localhost:8080/helloworld")
	if err != nil {
		t.Fatalf("Failed calling the helloworld endpoint in the sample API: %v", err)
	}

	t.Log("Received 200 response from the sample API")

	defer r.Body.Close()

	//We Read the response body on the line below.
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		t.Fatalf("Failed reading response body from the sample API: %v", err)
	}

	return string(body)
}