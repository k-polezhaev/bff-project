package models

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