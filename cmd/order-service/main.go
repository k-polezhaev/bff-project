package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
)

type Order struct {
	ID     string  `json:"id"`
	Amount float64 `json:"amount"`
	Status string  `json:"status"`
}

func main() {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /orders/user/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")

		fmt.Printf("Запрос профиля для ID: %s\n", id)

		orders := []Order{{ID: "5", Amount: 234, Status: "200"}}

		w.Header().Set("Content-Type", "application/json")

		if err := json.NewEncoder(w).Encode(orders); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	})

	log.Println("Сервер запущен на порту: 8082")

	if err := http.ListenAndServe(":8082", mux); err != nil {
		log.Fatal(err)
	}
}
