package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/go-redis/redis/v8"
)

var ctx = context.Background()

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
	Recommendations []Product `json:"recommendations"`
}

var (
	userServiceURL    = getEnv("USER_SERVICE_URL", "http://localhost:8081")
	orderServiceURL   = getEnv("ORDER_SERVICE_URL", "http://localhost:8082")
	productServiceURL = getEnv("PRODUCT_SERVICE_URL", "http://localhost:8083")
	redisAddr         = getEnv("REDIS_ADDR", "localhost:6379")
)

func getEnv(key, fallback string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return fallback
}

var rdb *redis.Client

func getUser(userID string) (*User, error) {
	resp, err := http.Get(userServiceURL + "/users/" + userID)
	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("user service returned status: %d", resp.StatusCode)
	}

	var user User
	if err := json.NewDecoder(resp.Body).Decode(&user); err != nil {
		return nil, err
	}

	return &user, nil
}

func getOrders(orderId string) ([]Order, error) {
	resp, err := http.Get(orderServiceURL + "/orders/user/" + orderId)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("order service error: %d", resp.StatusCode)
	}

	var orders []Order
	if err := json.NewDecoder(resp.Body).Decode(&orders); err != nil {
		return nil, err
	}
	return orders, nil
}

func getProducts() ([]Product, error) {
	resp, err := http.Get(productServiceURL + "/products")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("product service error: %d", resp.StatusCode)
	}

	var products []Product
	if err := json.NewDecoder(resp.Body).Decode(&products); err != nil {
		return nil, err
	}
	return products, nil
}

func main() {
	rdb = redis.NewClient(&redis.Options{
		Addr:     redisAddr,
		Password: "",
		DB:       0,
	})

	if _, err := rdb.Ping(ctx).Result(); err != nil {
		log.Fatalf("Ошибка подключения к Redis: %v", err)
	}
	log.Println("Успешное подключение к Redis!")

	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/profile/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		start := time.Now()
		cacheKey := fmt.Sprintf("profile:%s", id)

		cachedData, err := rdb.Get(ctx, cacheKey).Bytes()
		if err == nil {
			log.Printf("Кэш найден для ID: %s. Время: %v", id, time.Since(start))
			w.Header().Set("Content-Type", "application/json")
			w.Write(cachedData)
			return
		}

		if err != redis.Nil && err != nil {
			log.Printf("Ошибка Redis GET: %v", err)
		}

		user, err := getUser(id)
		if err != nil {
			log.Printf("Ошибка получения пользователя: %v", err)
			http.Error(w, "Failed to fetch user", http.StatusInternalServerError)
			return
		}

		var wg sync.WaitGroup
		var orders []Order
		var products []Product
		var ordersErr error
		var productsErr error

		wg.Add(2)

		// goroutine orders
		go func() {
			defer wg.Done()

			orders, ordersErr = getOrders(id)
			if ordersErr != nil {
				log.Printf("Goroutine Order Service error: %v", ordersErr)
			}
		}()

		// goroutine products
		go func() {
			defer wg.Done()

			products, productsErr = getProducts()
			if productsErr != nil {
				log.Printf("Goroutine Product Service error: %v", productsErr)
			}
		}()

		wg.Wait()

		if ordersErr != nil {
			orders = []Order{}
		}
		if productsErr != nil {
			products = []Product{}
		}

		response := ProfileResponse{
			User:            user,
			Orders:          orders,
			Recommendations: products,
		}

		responseBytes, err := json.Marshal(response)
		if err != nil {
			log.Printf("Ошибка JSON кодирования: %v", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		cacheDuration := 30 * time.Second
		if setErr := rdb.Set(ctx, cacheKey, responseBytes, cacheDuration).Err(); setErr != nil {
			log.Printf("Ошибка Redis SET: %v", setErr)
		}

		log.Printf("Запрос обработан и закэширован за %v", time.Since(start))
		w.Header().Set("Content-Type", "application/json")
		w.Write(responseBytes)
	})

	log.Println("API Gateway запущен на порту :8080")
	if err := http.ListenAndServe(":8080", mux); err != nil {
		log.Fatal(err)
	}
}
