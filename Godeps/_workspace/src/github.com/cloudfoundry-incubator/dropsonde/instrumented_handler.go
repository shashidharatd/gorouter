package dropsonde

import (
	"github.com/cloudfoundry-incubator/dropsonde-common/emitter"
	"github.com/cloudfoundry-incubator/dropsonde-common/events"
	"github.com/cloudfoundry-incubator/dropsonde-common/factories"
	uuid "github.com/nu7hatch/gouuid"
	"net/http"
)

type instrumentedHandler struct {
	handler http.Handler
	emitter emitter.EventEmitter
}

/*
Helper for creating an Instrumented Handler which will delegate to the given http.Handler.
*/
func InstrumentedHandler(handler http.Handler, emitter emitter.EventEmitter) http.Handler {
	return &instrumentedHandler{handler, emitter}
}

/*
Wraps the given http.Handler ServerHTTP function
Will provide accounting metrics for the http.Request / http.Response life-cycle
*/
func (ih *instrumentedHandler) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	requestId, err := uuid.ParseHex(req.Header.Get("X-CF-RequestID"))
	if err != nil {
		requestId, err = uuid.NewV4()
		if err != nil {
			panic(err)
		}
		req.Header.Set("X-CF-RequestID", requestId.String())
	}
	rw.Header().Set("X-CF-RequestID", requestId.String())

	startEvent := factories.NewHttpStart(req, events.PeerType_Server, requestId)
	ih.emitter.Emit(startEvent)

	instrumentedWriter := &instrumentedResponseWriter{writer: rw, statusCode: 200}
	ih.handler.ServeHTTP(instrumentedWriter, req)

	stopEvent := factories.NewHttpStop(instrumentedWriter.statusCode, instrumentedWriter.contentLength,
		events.PeerType_Server, requestId)

	ih.emitter.Emit(stopEvent)
}

type instrumentedResponseWriter struct {
	writer        http.ResponseWriter
	contentLength int64
	statusCode    int
}

func (irw *instrumentedResponseWriter) Header() http.Header {
	return irw.writer.Header()
}

func (irw *instrumentedResponseWriter) Write(data []byte) (int, error) {
	writeCount, err := irw.writer.Write(data)
	irw.contentLength += int64(writeCount)
	return writeCount, err
}

func (irw *instrumentedResponseWriter) WriteHeader(statusCode int) {
	irw.statusCode = statusCode
	irw.writer.WriteHeader(statusCode)
}

func (irw *instrumentedResponseWriter) Flush() {
	flusher, ok := irw.writer.(http.Flusher)

	if !ok {
		panic("Called Flush on an InstrumentedResponseWriter that wraps a non-Flushable writer.")
	}

	flusher.Flush()
}