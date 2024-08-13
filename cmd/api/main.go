package main

import (
	"context"
	"fmt"
	"net/http"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/tonkeeper/tongo/ton"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"github.com/tonkeeper/claim-api-go/pkg/api"
)

func createLogger(level string) (*zap.Logger, error) {
	cfg := zap.NewProductionConfig()
	if level != "" {
		lvl, err := zapcore.ParseLevel(level)
		if err != nil {
			return nil, err
		}
		cfg.Level.SetLevel(lvl)
	}
	return cfg.Build()
}

func main() {
	cfg := Load()
	logger, err := createLogger(cfg.App.LogLevel)
	if err != nil {
		logger.Fatal("createLogger() failed", zap.Error(err))
	}

	jettonMaster, err := ton.ParseAccountID(cfg.App.JettonMaster)
	if err != nil {
		logger.Fatal("failed to parse jetton master", zap.Error(err))
	}

	conf := api.Config{
		AirdropFilename: cfg.App.AirdropDataBocFilename,
		JettonMaster:    jettonMaster,
	}

	handler, err := api.NewHandler(logger, conf)
	if err != nil {
		logger.Fatal("api.NewHandler() failed", zap.Error(err))
	}
	handler.Run(context.TODO())
	server, err := api.NewServer(logger, handler, fmt.Sprintf(":%v", cfg.API.Port))
	if err != nil {
		logger.Fatal("api.NewServer() failed", zap.Error(err))
	}
	metricServer := http.Server{
		Addr:    fmt.Sprintf(":%v", cfg.API.MetricsPort),
		Handler: promhttp.Handler(),
	}
	go func() {
		if err := metricServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatal("listen and serve", zap.Error(err))
		}
	}()

	fmt.Printf("running server :%v\n", cfg.API.Port)
	server.Run()
}
