package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/go-redis/redis/v8"
)

var (
	userServiceURL           = getEnv("USER_SERVICE_URL", "http://localhost:8081")
	orderServiceURL          = getEnv("ORDER_SERVICE_URL", "http://localhost:8082")
	productServiceURL        = getEnv("PRODUCT_SERVICE_URL", "http://localhost:8083")
	recommendationServiceURL = getEnv("RECOMMENDATION_SERVICE_URL", "http://localhost:8084")
	redisAddr                = getEnv("REDIS_ADDR", "localhost:6379")
)

var ctx = context.Background()
var rdb *redis.Client

var httpClient = &http.Client{
	Timeout: 5 * time.Second,
}

type User struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
}

type Order struct {
	ID     string  `json:"id"`
	Amount float64 `json:"amount"`
	Status string  `json:"status"`
}

type Product struct {
	ID    string  `json:"id"`
	Name  string  `json:"name"`
	Price float64 `json:"price"`
}

type ProfileResponse struct {
	User            *User     `json:"user"`
	Orders          []Order   `json:"orders"`
	Products        []Product `json:"products"`
	Recommendations []Product `json:"recommendations"`
}

type State int

const (
	StateClosed State = iota
	StateOpen
	StateHalfOpen
)

type CircuitBreaker struct {
	mu            sync.Mutex
	state         State
	failureCount  int
	lastErrorTime time.Time
	threshold     int
	timeout       time.Duration
}

func NewCircuitBreaker(threshold int, timeout time.Duration) *CircuitBreaker {
	return &CircuitBreaker{
		state:     StateClosed,
		threshold: threshold,
		timeout:   timeout,
	}
}

func (cb *CircuitBreaker) Execute(action func() (interface{}, error)) (interface{}, error) {
	cb.mu.Lock()

	switch cb.state {
	case StateOpen:
		if time.Since(cb.lastErrorTime) > cb.timeout {
			cb.state = StateHalfOpen
		} else {
			cb.mu.Unlock()
			return nil, errors.New("circuit breaker is open")
		}
	case StateHalfOpen:
		cb.mu.Unlock()
		return nil, errors.New("circuit breaker is half-open (rate limited)")
	}

	cb.mu.Unlock()

	result, err := action()

	cb.mu.Lock()
	defer cb.mu.Unlock()

	if err != nil {
		cb.failureCount++
		cb.lastErrorTime = time.Now()

		if cb.failureCount >= cb.threshold || cb.state == StateHalfOpen {
			cb.state = StateOpen
			log.Printf("!!! CB OPENED: Service failed %d times or failed recovery !!!", cb.failureCount)
		}
		return nil, err
	}

	if cb.state == StateHalfOpen {
		log.Println("!!! CB RECOVERED: System healthy again !!!")
	}
	cb.failureCount = 0
	cb.state = StateClosed

	return result, nil
}

var recommendationsCB = NewCircuitBreaker(3, 10*time.Second)

func getEnv(key, fallback string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return fallback
}

func isRateLimited(ip string) bool {
	key := fmt.Sprintf("ratelimit:%s", ip)
	limitWindow := 60 * time.Second
	maxRequests := 10

	pipe := rdb.Pipeline()
	incr := pipe.Incr(ctx, key)
	pipe.Expire(ctx, key, limitWindow)
	_, err := pipe.Exec(ctx)

	if err != nil {
		log.Printf("Redis RateLimit error: %v", err)
		return false
	}

	return incr.Val() > int64(maxRequests)
}

func fetchJSON(url string, target interface{}) error {
	resp, err := httpClient.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("bad status code: %d", resp.StatusCode)
	}

	return json.NewDecoder(resp.Body).Decode(target)
}

func profileHandler(w http.ResponseWriter, r *http.Request) {
	clientIP := r.RemoteAddr
	if idx := strings.LastIndex(clientIP, ":"); idx != -1 {
		clientIP = clientIP[:idx]
	}

	if isRateLimited(clientIP) {
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

	cachedData, err := rdb.Get(ctx, cacheKey).Bytes()
	if err == nil {
		log.Printf("Cache HIT for %s (%v)", id, time.Since(start))
		w.Header().Set("Content-Type", "application/json")
		w.Write(cachedData)
		return
	}

	var (
		wg              sync.WaitGroup
		user            User
		orders          []Order
		products        []Product
		recommendations []Product
		userErr         error
	)

	userErr = fetchJSON(fmt.Sprintf("%s/users/%s", userServiceURL, id), &user)
	if userErr != nil {
		log.Printf("Failed to get user: %v", userErr)
		http.Error(w, "User not found or service unavailable", http.StatusNotFound)
		return
	}

	wg.Add(3)

	go func() {
		defer wg.Done()
		err := fetchJSON(fmt.Sprintf("%s/orders/user/%s", orderServiceURL, id), &orders)
		if err != nil {
			log.Printf("Orders fetch error: %v", err)
			orders = []Order{}
		}
	}()

	go func() {
		defer wg.Done()
		url := fmt.Sprintf("%s/products", productServiceURL)

		err := fetchJSON(url, &products)
		if err != nil {
			log.Printf("Products error: %v", err)
			products = []Product{}
		}
	}()

	go func() {
		defer wg.Done()
		url := fmt.Sprintf("%s/recommendations/%s", recommendationServiceURL, id)

		result, err := recommendationsCB.Execute(func() (any, error) {
			var recs []Product
			if err := fetchJSON(url, &recs); err != nil {
				return nil, err
			}
			return recs, nil
		})

		if err != nil {
			log.Printf("Recommendations fallback (CB/Error): %v", err)
			recommendations = []Product{}
		} else {
			recommendations = result.([]Product)
		}
	}()

	wg.Wait()

	response := ProfileResponse{
		User:            &user,
		Orders:          orders,
		Products:        products,
		Recommendations: recommendations,
	}

	responseBytes, err := json.Marshal(response)
	if err != nil {
		http.Error(w, "JSON marshal error", http.StatusInternalServerError)
		return
	}

	rdb.Set(ctx, cacheKey, responseBytes, 30*time.Second)

	log.Printf("Request processed in %v", time.Since(start))
	w.Header().Set("Content-Type", "application/json")
	w.Write(responseBytes)
}

func main() {
	rdb = redis.NewClient(&redis.Options{
		Addr: redisAddr,
	})

	if _, err := rdb.Ping(ctx).Result(); err != nil {
		log.Fatalf("Redis connection failed: %v", err)
	}
	log.Println("Connected to Redis")

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/profile/{id}", profileHandler)

	log.Println("Gateway running on :8080")
	if err := http.ListenAndServe(":8080", mux); err != nil {
		log.Fatal(err)
	}
}
