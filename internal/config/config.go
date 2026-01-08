package config

import (
	"os"
)

type Config struct {
	UserServiceURL           string
	OrderServiceURL          string
	ProductServiceURL        string
	RecommendationServiceURL string
	RedisAddr                string
	HTTPPort                 string
}

func NewConfig() *Config {
	return &Config{
		UserServiceURL:           getEnv("USER_SERVICE_URL", "http://localhost:8081"),
		OrderServiceURL:          getEnv("ORDER_SERVICE_URL", "http://localhost:8082"),
		ProductServiceURL:        getEnv("PRODUCT_SERVICE_URL", "http://localhost:8083"),
		RecommendationServiceURL: getEnv("RECOMMENDATION_SERVICE_URL", "http://localhost:8084"),
		RedisAddr:                getEnv("REDIS_ADDR", "localhost:6379"),
		HTTPPort:                 getEnv("HTTP_PORT", "8080"),
	}
}

func getEnv(key, fallback string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return fallback
}