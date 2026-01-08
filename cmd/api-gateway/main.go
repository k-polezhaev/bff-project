package main

import (
	"fmt"
	"log/slog"
	"net/http"
	"os"

	"bff-project/internal/api"
	"bff-project/internal/cache"
	"bff-project/internal/config"
	"bff-project/internal/services"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	cfg := config.NewConfig()
	slog.Info("Starting API Gateway", "port", cfg.HTTPPort)

	redisClient, err := cache.NewClient(cfg.RedisAddr)
	if err != nil {
		slog.Error("Failed to connect to Redis", "error", err)
		os.Exit(1)
	}
	defer redisClient.Close()
	slog.Info("Connected to Redis", "addr", cfg.RedisAddr)

	serviceClient := services.NewServiceClient(cfg)

	handler := api.NewHandler(serviceClient, redisClient)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/profile/{id}", handler.GetProfile)

	serverAddr := fmt.Sprintf(":%s", cfg.HTTPPort)
	slog.Info("Server listening", "addr", serverAddr)
	
	if err := http.ListenAndServe(serverAddr, mux); err != nil {
		slog.Error("Server shutdown error", "error", err)
		os.Exit(1)
	}
}