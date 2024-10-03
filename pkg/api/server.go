package api

import (
	"errors"
	"net/http"

	"github.com/rs/cors"
	"go.uber.org/zap"

	"github.com/tonkeeper/claim-api-go/pkg/api/oas"
)

type Server struct {
	logger     *zap.Logger
	httpServer *http.Server
}

func NewServer(log *zap.Logger, handler *Handler, address string) (*Server, error) {
	ogenMiddlewares := []oas.Middleware{ogenLoggingMiddleware(log), ogenMetricsMiddleware}
	ogenServer, err := oas.NewServer(handler,
		oas.WithMiddleware(ogenMiddlewares...))

	if err != nil {
		return nil, err
	}
	mux := http.NewServeMux()
	mux.Handle("/", ogenServer)
	mux.HandleFunc("/healthz", healthzHandler())

	serv := Server{
		logger: log,
		httpServer: &http.Server{
			Addr:    address,
			Handler: cors.AllowAll().Handler(mux),
		},
	}
	return &serv, nil
}

func (s *Server) Run() {
	err := s.httpServer.ListenAndServe()
	if errors.Is(err, http.ErrServerClosed) {
		s.logger.Info("claim-api-go quit")
		return
	}
	s.logger.Fatal("ListedAndServe() failed", zap.Error(err))
}

func healthzHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}
}
