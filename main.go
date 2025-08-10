package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/ovh/go-ovh/ovh"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

type record struct { // for client requests
	FieldType string `json:"fieldType"`
	Subdomain string `json:"subDomain"`
	Target    string `json:"target"`
	Ttl       int    `json:"ttl"`
}

type recAndID struct { // for our inner usage
	FieldType string
	Subdomain string
	Target    string
	Ttl       int
	Id        int
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

func IDToRecord(client *ovh.Client, id int) (record, error) {
	var info record
	err := client.Get(fmt.Sprintf("/domain/zone/%s/record/%d", os.Getenv("DOMAIN"), id), &info)
	if err != nil {
		return record{}, err
	}
	return info, nil
}

func PostNewRecord(client *ovh.Client, rec record) error {
	var resp record
	err := client.Post(fmt.Sprintf("/domain/zone/%s/record", os.Getenv("DOMAIN")), rec, &resp)
	if err != nil {
		return err
	}

	err = RefreshZone(client)
	if err != nil {
		return err
	}
	return nil
}

func UpdateRecord(client *ovh.Client, rec record, id int) error {
	var resp record
	err := client.Put(fmt.Sprintf("/domain/zone/%s/record/%d", os.Getenv("DOMAIN"), id), rec, resp)
	if err != nil {
		return err
	}
	return nil
}

func RefreshZone(client *ovh.Client) error {
	err := client.Post(fmt.Sprintf("/domain/zone/%s/refresh", os.Getenv("DOMAIN")), nil, nil)
	if err != nil {
		return err
	}
	return nil
}

func NewRecord(fieldType string, subDomain string, target string, ttl int) *record {
	return &record{
		FieldType: fieldType,
		Subdomain: subDomain,
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

func PollRecords(client *ovh.Client, fieldType string, pubIP string) ([]recAndID, error) {
	var recordsIDs []int
	var records []recAndID
	err := client.Get(fmt.Sprintf("/domain/zone/%s/record?fieldType=%s", os.Getenv("DOMAIN"), fieldType), &recordsIDs)
	if err != nil {
		return nil, err
	}
	for _, id := range recordsIDs {
		rec, err := IDToRecord(client, id)
		if err != nil {
			log.Error().Err(err).Msgf("Failed to retrieve info for record ID : %d", id)
		} else {
			if rec.Target == pubIP {
				records = append(records, recAndID{
					FieldType: rec.FieldType,
					Subdomain: rec.Subdomain,
					Target:    rec.Target,
					Ttl:       rec.Ttl,
					Id:        id,
				})
				log.Debug().Str("type", rec.FieldType).Str("Subdomain", rec.Subdomain).Str("IP", rec.Target).Msg("Matching record found")
			}
		}
	}
	return records, nil
}

func main() {
	zerolog.SetGlobalLevel(zerolog.DebugLevel)
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})

	ctx, cancel := context.WithCancel(context.Background())

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigs
		log.Info().Str("signal", sig.String()).Msg("Received termination signal")
		cancel()
	}()

	client, err := NewOVHClient()
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to create OVH client")
	}

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

	var previousA []recAndID    // records with public IPv4
	var previousAAAA []recAndID // same for IPv6

	for {
		select {
		case <-time.After(time.Duration(timeInterval) * time.Second):
			// Poll IPs
			pubIPv4, err := getPublicIP(false)
			if err != nil {
				log.Error().Err(err).Msg("Failed to get public IPv4")
			}
			pubIPv6, err := getPublicIP(true)
			if err != nil {
				log.Error().Err(err).Msg("Failed to get public IPv6")
			}

			// IPv4 check
			ARecords, err := PollRecords(client, "A", pubIPv4)
			if err != nil {
				log.Error().Err(err).Msg("Failed to get A records list")
				continue
			}
			if len(ARecords) == 0 {
				// We need to update the previously found record
				if len(previousA) == 0 {
					rec := NewRecord("A", "", pubIPv4, 0)
					err = PostNewRecord(client, *rec)
					if err != nil {
						log.Error().Err(err).Str("IP", rec.Target).Int("TTL", rec.Ttl).Msg("Failed to add record")
					} else {
						log.Info().Str("IP", rec.Target).Int("TTL", rec.Ttl).Msg("Sucessfully added record")
					}
				} else {
					for _, prev := range previousA {
						rec := NewRecord(prev.FieldType, prev.Subdomain, pubIPv4, prev.Ttl)
						err = UpdateRecord(client, *rec, prev.Id)
						if err != nil {
							log.Error().Err(err).Str("type", prev.FieldType).Str("Subdomain", prev.Subdomain).Str("IP", prev.Target).Msg("Failed to update record")
						} else {
							log.Debug().Int("ID", prev.Id).Str("Type", prev.FieldType).Str("Subdomain", prev.Subdomain).Msg("Sucessfully updated record")
						}
					}
					err = RefreshZone(client)
					if err != nil {
						log.Error().Err(err).Msg("Failed to refresh DNS zone")
					}
				}
			} else {
				previousA = ARecords
				log.Info().Msg("Public IPv4 sucessfully found in A record")
			}

			// IPv6 check
			AAAARecords, err := PollRecords(client, "AAAA", pubIPv6)
			for _, t := range AAAARecords {
				log.Debug().Msgf("%s in AAAA records", t.Subdomain)
			}
			if len(AAAARecords) == 0 {
				// We need to update the previously found record
				if len(previousAAAA) == 0 {
					rec := NewRecord("AAAA", "", pubIPv6, 0)
					err = PostNewRecord(client, *rec)
					if err != nil {
						log.Error().Err(err).Str("IP", rec.Target).Int("TTL", rec.Ttl).Msg("Failed to add record")
					} else {
						log.Info().Str("IP", rec.Target).Int("TTL", rec.Ttl).Msg("Sucessfully added record")
					}
				} else {
					for _, prev := range previousAAAA {
						rec := NewRecord(prev.FieldType, prev.Subdomain, pubIPv6, prev.Ttl)
						err = UpdateRecord(client, *rec, prev.Id)
						if err != nil {
							log.Error().Err(err).Str("type", rec.FieldType).Str("Subdomain", rec.Subdomain).Str("IP", rec.Target).Msg("Failed to update record")
						} else {
							log.Debug().Int("ID", prev.Id).Str("Type", prev.FieldType).Str("Subdomain", prev.Subdomain).Msg("Sucessfully updated record")
						}
					}
					err = RefreshZone(client)
					if err != nil {
						log.Error().Err(err).Msg("Failed to refresh DNS zone")
					}
				}
			} else {
				previousAAAA = AAAARecords
				log.Info().Msg("Public IPv6 sucessfully found in A record")
			}
		case <-ctx.Done():
			log.Info().Msg("Closing program")
			return
		}
	}
}
