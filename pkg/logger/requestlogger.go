package logger

import (
	"fmt"
	"log/slog"
	"net/http"
	"time"
)

type headerSniffer struct {
	header int
	writer http.ResponseWriter
}

func (h *headerSniffer) Header() http.Header {
	return h.writer.Header()
}

func (h *headerSniffer) Write(b []byte) (int, error) {
	return h.writer.Write(b)
}
func (h *headerSniffer) WriteHeader(statusCode int) {
	h.header = statusCode
	h.writer.WriteHeader(statusCode)
}

func AccessLogger(next http.Handler, logger slog.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		startTime := time.Now()

		rw := &headerSniffer{
			writer: w,
		}
		ctx := r.Context()
		fmt.Println(time.Since(startTime))
		defer func() {
			logger.InfoContext(
				ctx,
				r.URL.String(),
				slog.String("http_method", r.Method),
				slog.String("remote_addr", r.RemoteAddr),
				slog.String("user_agent", r.UserAgent()),
				slog.Int("status", rw.header),
				slog.Int64("response_duration_ns", time.Since(startTime).Nanoseconds()),
			)
		}()

		next.ServeHTTP(rw, r)
	})
}
