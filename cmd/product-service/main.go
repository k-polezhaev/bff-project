package main

import (
	"encoding/json"
	"log"
	"net/http"
)

type Product struct {
	ID    string  `json:"id"`
	Name  string  `json:"name"`
	Price float64 `json:"price"`
}

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /products", func(w http.ResponseWriter, r *http.Request) {

		products := []Product{{ID: "4", Name: "Sam", Price: 543}}

		w.Header().Set("Content-Type", "application/json")

		if err := json.NewEncoder(w).Encode(products); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}

	})
	log.Println("Сервер запущен на порту: 8083")

	if err := http.ListenAndServe(":8083", mux); err != nil {
		log.Fatal(err)
	}
}
