package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"time"
	"strconv"

	"github.com/ovh/go-ovh/ovh"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

func getEnv(key string) (string, error) {
	value := os.Getenv(key)

	if value == "" {
		return "", fmt.Errorf("%s environment variable is required", key)
	}

	return value, nil
}

func getPublicIP() (string, error) {
	resp, err := http.Get("https://api.ipify.org?format=json")
	if err != nil {
		return "", err
	}

	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to get public IP, status code : %d", resp.StatusCode)
	}

	var result map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}

	ip := result["ip"]
	if ip == "" {
		return "", errors.New("public IP not found in response")
	}

	log.Info().Str("ip", ip).Msg("Public IP found")
	return ip, nil
}

func NewOVHClient() (*ovh.Client, error) {
	endpoint, err := getEnv("OVH_ENDPOINT")
	if err != nil {
		return nil, err
	}

	appKey, err := getEnv("OVH_APP_KEY")
	if err != nil {
		return nil, err
	}

	appSecret, err := getEnv("OVH_APP_SECRET")
	if err != nil {
		return nil, err
	}
	
	consumerKey, err := getEnv("OVH_CONSUMER_KEY")
	if err != nil {
		return nil, err
	}

	client, err := ovh.NewClient(endpoint, appKey, appSecret, consumerKey)
	if err != nil {
		return nil, err
	}

	return client, nil

}

func main() {
	zerolog.SetGlobalLevel(zerolog.DebugLevel)
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})

	client, err := NewOVHClient()
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to create OVH client")
	}

	_, err = getPublicIP()
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to get public IP")
	}

	type PartialMe struct {
		Firstname string `json:"firstname"`
	}

	var me PartialMe
	err = client.Get("/me", &me)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to make request to OVH API")
	}
	log.Info().Msg("Successfully established connection to OVH API")

	var recordsID []int

	interval, err := getEnv("TIME_INTERVAL")
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to get time interval")
	}
	timeInterval, err := strconv.Atoi(interval)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to convert time interval to int")
	}
	
	ticker := time.NewTicker(time.Duration(timeInterval) * time.Second)
	defer ticker.Stop()

	for {
		err = client.Get("/domain/zone/" + os.Getenv("DOMAIN") + "/record?fieldType=A", &recordsID)
		if err != nil {
			log.Error().Err(err).Msg("Failed to get A records list")
			continue
		}
		log.Info().Msg("Record list found")
		<-ticker.C
	}
}
