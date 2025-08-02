package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"slices"
	"strconv"
	"time"

	"github.com/ovh/go-ovh/ovh"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

type record struct {
	FieldType string `json:"fieldType"`
	Subdomain string `json:"subDomain"`
	Target    string `json:"target"`
	Ttl       int    `json:"ttl"`
}

func getEnv(key string) (string, error) {
	value := os.Getenv(key)

	if value == "" {
		return "", fmt.Errorf("%s environment variable is required", key)
	}

	return value, nil
}

func getPublicIP(v6 bool) (string, error) {
	var resp *http.Response
	var err error
	if v6 {
		resp, err = http.Get("https://api6.ipify.org?format=json")
	} else {
		resp, err = http.Get("https://api.ipify.org?format=json")
	}

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

func IDToIP(client *ovh.Client, id int) (string, error) {
	var info record
	err := client.Get(fmt.Sprintf("/domain/zone/%s/record/%d", os.Getenv("DOMAIN"), id), &info)
	if err != nil {
		return "", err
	}
	return info.Target, nil
}

func addRecord(client *ovh.Client, rec record) error {
	var resp record
	err := client.Post(fmt.Sprintf("/domain/zone/%s/record", os.Getenv("DOMAIN")), rec, &resp)
	if err != nil {
		return err
	}
	err = client.Post(fmt.Sprintf("/domain/zone/%s/refresh", os.Getenv("DOMAIN")), nil, nil)
	if err != nil {
		return err
	}
	return nil
}

func NewRecord(fieldType string, target string, ttl int) *record {
	return &record{
		FieldType: fieldType,
		Target:    target,
		Ttl:       ttl,
	}
}

func ConnAttempt(client *ovh.Client) error {
	type PartialMe struct {
		Firstname string `json:"firstname"`
	}

	var me PartialMe
	err := client.Get("/me", &me)
	if err != nil {
		return err
	}
	return nil
}

func PollRecords(client *ovh.Client, fieldType string) ([]string, error) {
	var recordsIDs []int
	var IPs []string
	err := client.Get(fmt.Sprintf("/domain/zone/%s/record?fieldType=%s", os.Getenv("DOMAIN"), fieldType), &recordsIDs)
	if err != nil {
		return nil, err
	}
	for _, record := range recordsIDs {
		log.Debug().Int("ID", record).Msg("Record ID found")
		ip, err := IDToIP(client, record)
		if err != nil {
			log.Error().Err(err).Msgf("Failed to retrieve info for record ID : %d", record)
		} else {
			IPs = append(IPs, ip)
			log.Debug().Msgf("ID : %d -> IP : %s", record, ip)
		}
	}
	return IPs, nil
}

func main() {
	zerolog.SetGlobalLevel(zerolog.DebugLevel)
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})

	client, err := NewOVHClient()
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to create OVH client")
	}

	pubIPv4, err := getPublicIP(false)
	if err != nil {
		log.Error().Err(err).Msg("Failed to get public IPv4")
	}
	pubIPv6, err := getPublicIP(true)
	if err != nil {
		log.Error().Err(err).Msg("Failed to get public IPv6")
	}

	log.Info().Str("ip", pubIPv4).Msg("Public IPv4 found")

	err = ConnAttempt(client)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to estanlished connection to OVH API")
	}
	log.Info().Msg("Successfully established connection to OVH API")

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
		// IPv4 check
		IPv4List, err := PollRecords(client, "A")
		if err != nil {
			log.Error().Err(err).Msg("Failed to get A records list")
			continue
		}
		if slices.Contains(IPv4List, pubIPv4) {
			log.Info().Msg("Public IPv4 sucessfully found in A record")
		} else {
			rec := NewRecord("A", pubIPv4, 0)
			err = addRecord(client, *rec)
			if err != nil {
				log.Error().Err(err).Str("IP", rec.Target).Int("TTL", rec.Ttl).Msg("Failed to add record")
			} else {
				log.Info().Str("IP", rec.Target).Int("TTL", rec.Ttl).Msg("Sucessfully added record")
			}
		}
		// IPv6 check
		IPv6List, err := PollRecords(client, "AAAA")
		if err != nil {
			log.Error().Err(err).Msg("Failed to get AAAA records list")
			continue
		}
		if slices.Contains(IPv6List, pubIPv6) {
			log.Info().Msg("Public IPv6 sucessfully found in AAAA record")
		} else {
			rec := NewRecord("AAAA", pubIPv6, 0)
			err := addRecord(client, *rec)
			if err != nil {
				log.Error().Err(err).Str("IP", rec.Target).Int("TTL", rec.Ttl).Msg("Failed to add record")
			} else {
				log.Info().Str("IP", rec.Target).Int("TTL", rec.Ttl).Msg("Sucessfully added record")
			}
		}
		<-ticker.C
	}
}
