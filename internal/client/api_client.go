package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"aka-webgui/internal/config"
)

type Subscriber struct {
	IMSI      string    `json:"imsi"`
	Ki        string    `json:"ki,omitempty"`
	Opc       string    `json:"opc,omitempty"`
	Sqn       string    `json:"sqn"`
	Amf       string    `json:"amf"`
	CreatedAt time.Time `json:"created_at"`
}

type CountResponse struct {
	Count int `json:"count"`
}

type Client struct {
	BaseURL    string
	HTTPClient *http.Client
}

func New(cfg *config.Config) *Client {
	return &Client{
		BaseURL: cfg.AKABaseURL,
		HTTPClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

func (c *Client) GetSubscriberCount() (int, error) {
	resp, err := c.HTTPClient.Get(c.BaseURL + "/subscribers/count")
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("failed to get count: %s", resp.Status)
	}

	var cr CountResponse
	if err := json.NewDecoder(resp.Body).Decode(&cr); err != nil {
		return 0, err
	}
	return cr.Count, nil
}

func (c *Client) GetSubscribers() ([]Subscriber, error) {
	resp, err := c.HTTPClient.Get(c.BaseURL + "/subscribers")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to get subscribers: %s", resp.Status)
	}

	var subs []Subscriber
	if err := json.NewDecoder(resp.Body).Decode(&subs); err != nil {
		return nil, err
	}
	return subs, nil
}

func (c *Client) GetSubscriber(imsi string) (*Subscriber, error) {
	resp, err := c.HTTPClient.Get(fmt.Sprintf("%s/subscribers/%s", c.BaseURL, imsi))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to get subscriber: %s", resp.Status)
	}

	var sub Subscriber
	if err := json.NewDecoder(resp.Body).Decode(&sub); err != nil {
		return nil, err
	}
	return &sub, nil
}

func (c *Client) CreateSubscriber(sub *Subscriber) error {
	data, err := json.Marshal(sub)
	if err != nil {
		return err
	}

	resp, err := c.HTTPClient.Post(c.BaseURL+"/subscribers", "application/json", bytes.NewBuffer(data))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("failed to create subscriber: %s", resp.Status)
	}
	return nil
}

func (c *Client) UpdateSubscriber(imsi string, sub *Subscriber) error {
	data, err := json.Marshal(sub)
	if err != nil {
		return err
	}

	req, err := http.NewRequest(http.MethodPut, fmt.Sprintf("%s/subscribers/%s", c.BaseURL, imsi), bytes.NewBuffer(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to update subscriber: %s", resp.Status)
	}
	return nil
}

func (c *Client) DeleteSubscriber(imsi string) error {
	req, err := http.NewRequest(http.MethodDelete, fmt.Sprintf("%s/subscribers/%s", c.BaseURL, imsi), nil)
	if err != nil {
		return err
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("failed to delete subscriber: %s", resp.Status)
	}
	return nil
}
