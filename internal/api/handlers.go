package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"bff-project/internal/cache"
	"bff-project/internal/models"
	"bff-project/internal/services"
)

type Handler struct {
	svc   *services.ServiceClient
	cache *cache.Client
}

func NewHandler(svc *services.ServiceClient, cache *cache.Client) *Handler {
	return &Handler{
		svc:   svc,
		cache: cache,
	}
}

func (h *Handler) GetProfile(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	clientIP := r.RemoteAddr
	if idx := strings.LastIndex(clientIP, ":"); idx != -1 {
		clientIP = clientIP[:idx]
	}

	if h.cache.IsRateLimited(ctx, clientIP) {
		slog.Warn("Rate limit exceeded", "ip", clientIP)
		http.Error(w, `{"error": "Too many requests"}`, http.StatusTooManyRequests)
		return
	}

	id := r.PathValue("id")
	if id == "" {
		parts := strings.Split(r.URL.Path, "/")
		if len(parts) > 0 {
			id = parts[len(parts)-1]
		}
	}

	cacheKey := fmt.Sprintf("profile:%s", id)
	start := time.Now()

	cachedData, err := h.cache.Get(ctx, cacheKey)
	if err == nil {
		slog.Info("Cache HIT", "user_id", id, "duration", time.Since(start))
		w.Header().Set("Content-Type", "application/json")
		w.Write(cachedData)
		return
	}

	user, err := h.svc.GetUser(id)
	if err != nil {
		slog.Error("Failed to get user", "user_id", id, "error", err)
		http.Error(w, "User not found or service unavailable", http.StatusNotFound)
		return
	}

	var (
		wg              sync.WaitGroup
		orders          []models.Order
		products        []models.Product
		recommendations []models.Product
	)

	wg.Add(3)

	go func() {
		defer wg.Done()
		res, err := h.svc.GetOrders(id)
		if err != nil {
			slog.Error("Orders fetch error", "user_id", id, "error", err)
			orders = []models.Order{} 
		} else {
			orders = res
		}
	}()

	go func() {
		defer wg.Done()
		res, err := h.svc.GetProducts()
		if err != nil {
			slog.Error("Products error", "error", err)
			products = []models.Product{}
		} else {
			products = res
		}
	}()

	go func() {
		defer wg.Done()
		res, err := h.svc.GetRecommendations(id)
		if err != nil {
			slog.Warn("Recommendations fallback", "user_id", id, "error", err)
			recommendations = []models.Product{}
		} else {
			recommendations = res
		}
	}()

	wg.Wait()

	response := models.ProfileResponse{
		User:            user,
		Orders:          orders,
		Products:        products,
		Recommendations: recommendations,
	}

	responseBytes, err := json.Marshal(response)
	if err != nil {
		slog.Error("JSON marshal error", "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	go func() {
		_ = h.cache.Set(context.Background(), cacheKey, responseBytes, 30*time.Second)
	}()

	slog.Info("Request processed", "user_id", id, "duration", time.Since(start))
	w.Header().Set("Content-Type", "application/json")
	w.Write(responseBytes)
}