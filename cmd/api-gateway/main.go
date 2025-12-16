package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"sync"
	"time"
)

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
)

func getEnv(key, fallback string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return fallback
}

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
	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/profile/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")

		log.Printf("Gateway: Запрос профиля для user_id=%s", id)

		start := time.Now()

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

		go func() {
			defer wg.Done()

			orders, ordersErr = getOrders(id)
			if ordersErr != nil {
				log.Printf("Goroutine Order Service error: %v", ordersErr)
			}
		}()

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

		log.Printf("Запрос обработан за %v", time.Since(start))

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(response); err != nil {
			log.Printf("Ошибка кодирования ответа: %v", err)
		}
	})

	log.Println("API Gateway запущен на порту :8080")
	if err := http.ListenAndServe(":8080", mux); err != nil {
		log.Fatal(err)
	}
}
