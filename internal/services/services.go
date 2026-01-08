package services

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"bff-project/internal/config"
	"bff-project/internal/models"
	"bff-project/internal/resilience"
)

type ServiceClient struct {
	cfg              *config.Config
	client           *http.Client
	recommendationCB *resilience.CircuitBreaker
}

func NewServiceClient(cfg *config.Config) *ServiceClient {
	return &ServiceClient{
		cfg: cfg,
		client: &http.Client{
			Timeout: 5 * time.Second,
		},
		recommendationCB: resilience.NewCircuitBreaker(3, 10*time.Second),
	}
}

func (s *ServiceClient) fetchJSON(url string, target interface{}) error {
	return resilience.Retry(3, 500*time.Millisecond, func() error {
		resp, err := s.client.Get(url)
		if err != nil {
			return err
		}
		defer resp.Body.Close()

		if resp.StatusCode >= 500 {
			return fmt.Errorf("server error: %d", resp.StatusCode)
		}
		
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return fmt.Errorf("bad status code: %d", resp.StatusCode)
		}

		return json.NewDecoder(resp.Body).Decode(target)
	})
}

func (s *ServiceClient) GetUser(userID string) (*models.User, error) {
	url := fmt.Sprintf("%s/users/%s", s.cfg.UserServiceURL, userID)
	var user models.User
	if err := s.fetchJSON(url, &user); err != nil {
		return nil, err
	}
	return &user, nil
}

func (s *ServiceClient) GetOrders(userID string) ([]models.Order, error) {
	url := fmt.Sprintf("%s/orders/user/%s", s.cfg.OrderServiceURL, userID)
	var orders []models.Order
	if err := s.fetchJSON(url, &orders); err != nil {
		return nil, err
	}
	return orders, nil
}

func (s *ServiceClient) GetProducts() ([]models.Product, error) {
	url := fmt.Sprintf("%s/products", s.cfg.ProductServiceURL)
	var products []models.Product
	if err := s.fetchJSON(url, &products); err != nil {
		return nil, err
	}
	return products, nil
}


func (s *ServiceClient) GetRecommendations(userID string) ([]models.Product, error) {
	url := fmt.Sprintf("%s/recommendations/%s", s.cfg.RecommendationServiceURL, userID)

	result, err := s.recommendationCB.Execute(func() (any, error) {
		var recs []models.Product
		if err := s.fetchJSON(url, &recs); err != nil {
			return nil, err
		}
		return recs, nil
	})

	if err != nil {
		return nil, err
	}

	return result.([]models.Product), nil
}