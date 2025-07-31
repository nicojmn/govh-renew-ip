package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/ovh/go-ovh/ovh"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

type Arecord struct {
	FieldType string
	Id        int
	Subdomain string
	Target    string
	Ttl       int
	Zone      string
}

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

func IdToIp(client *ovh.Client, id int) (string, error) {
	var info Arecord
	err := client.Get(fmt.Sprintf("/domain/zone/%s/record/%d", os.Getenv("DOMAIN"), id), &info)
	if err != nil {
		return "", err
	}
	return info.Target, nil
}

func IpinRecordList(client *ovh.Client, list []int, pubIP string) bool {
	for _, rec := range list {
		recIP, err := IdToIp(client, rec)
		if err != nil {
			continue
		}
		if recIP == pubIP {
			return true
		}
	}
	return false
}

func main() {
	zerolog.SetGlobalLevel(zerolog.DebugLevel)
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})

	client, err := NewOVHClient()
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to create OVH client")
	}

	pubIP, err := getPublicIP()
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to get public IP")
	}

	log.Info().Str("ip", pubIP).Msg("Public IP found")

	type PartialMe struct {
		Firstname string `json:"firstname"`
	}

	var me PartialMe
	err = client.Get("/me", &me)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to make request to OVH API")
	}
	log.Info().Str("user", me.Firstname).Msg("Successfully established connection to OVH API")

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
		err = client.Get(fmt.Sprintf("/domain/zone/%s/record?fieldType=A", os.Getenv("DOMAIN")), &recordsID)
		if err != nil {
			log.Error().Err(err).Msg("Failed to get A records list")
			continue
		} else {
			for _, record := range recordsID {
				log.Info().Int("ID", record).Msg("Record ID found")
				ip, err := IdToIp(client, record)
				if err != nil {
					log.Error().Err(err).Msgf("Failed to retrieve info for record ID : %d", record)
				} else {
					log.Debug().Msgf("ID : %d -> IP : %s", record, ip)
				}
			}
			if IpinRecordList(client, recordsID, pubIP) {
				log.Info().Msg("Public IP sucessfully found in A record")
			} else {
				// TODO : add/update A record
			}
		}
		<-ticker.C
	}
}
