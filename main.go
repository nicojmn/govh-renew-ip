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

	"github.com/akamensky/argparse"
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

var domain string

// getEnv retrieves the value of the specified environment variable.
// If the environment variable is not set or is empty, it returns an error
// indicating that the variable is required.
//
// Parameters:
//   key - the name of the environment variable to retrieve.
//
// Returns:
//   The value of the environment variable, or an error if it is not set.
func getEnv(key string) (string, error) {
	value := os.Getenv(key)

	if value == "" {
		return "", fmt.Errorf("%s environment variable is required", key)
	}

	return value, nil
}

// getPublicIP retrieves the public IP address of the host machine.
// If v6 is true, it fetches the IPv6 address using the api6.ipify.org service.
// If v6 is false, it fetches the IPv4 address using the api.ipify.org service.
// The function returns the IP address as a string, or an error if the request fails,
// the response status is not OK, or the IP is not found in the response.
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

// NewOVHClient initializes and returns a new OVH API client using credentials
// and endpoint information retrieved from environment variables. It expects the
// following environment variables to be set: OVH_ENDPOINT, OVH_APP_KEY,
// OVH_APP_SECRET, and OVH_CONSUMER_KEY. If any variable is missing or invalid,
// an error is returned. On success, it returns a pointer to an ovh.Client.
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

// IDToRecord converts a record ID to a record struct by making a GET request
func IDToRecord(client *ovh.Client, id int) (record, error) {
	var info record
	err := client.Get(fmt.Sprintf("/domain/zone/%s/record/%d", domain, id), &info)
	if err != nil {
		return record{}, err
	}
	return info, nil
}

// PostNewRecord creates a new DNS record in the specified domain zone using the provided OVH client and record data.
// After successfully posting the new record, it refreshes the DNS zone to apply the changes.
// Returns an error if the record creation or zone refresh fails.
func PostNewRecord(client *ovh.Client, rec record) error {
	var resp record
	err := client.Post(fmt.Sprintf("/domain/zone/%s/record", domain), rec, &resp)
	if err != nil {
		return err
	}

	err = RefreshZone(client)
	if err != nil {
		return err
	}
	return nil
}

// UpdateRecord updates a DNS record in the specified domain zone using the provided OVH client.
// It sends a PUT request to the OVH API to update the record with the given ID and record data.
// Returns an error if the update operation fails.
func UpdateRecord(client *ovh.Client, rec record, id int) error {

	var resp record
	err := client.Put(fmt.Sprintf("/domain/zone/%s/record/%d", domain, id), rec, resp)
	if err != nil {
		return err
	}
	return nil
}

// RefreshZone triggers a refresh of the DNS zone for the specified domain using the provided OVH client.
// It sends a POST request to the OVH API to update the zone records.
// Returns an error if the API request fails.
func RefreshZone(client *ovh.Client) error {
	err := client.Post(fmt.Sprintf("/domain/zone/%s/refresh", domain), nil, nil)
	if err != nil {
		return err
	}
	return nil
}

// NewRecord creates and returns a new DNS record with the specified field type, subdomain, target, and TTL.
// Parameters:
//   - fieldType: The type of DNS record (e.g., "A", "CNAME").
//   - subDomain: The subdomain for the DNS record.
//   - target: The target value for the DNS record (e.g., IP address or domain).
//   - ttl: The time-to-live value for the DNS record.
// Returns:
//   - A pointer to the newly created record.
func NewRecord(fieldType string, subDomain string, target string, ttl int) *record {
	return &record{
		FieldType: fieldType,
		Subdomain: subDomain,
		Target:    target,
		Ttl:       ttl,
	}
}

// ConnAttempt tests the connection to the OVH API by retrieving the user's information.
// It sends a GET request to the "/me" endpoint and unmarshals the response into a PartialMe struct.
// If the request fails, it returns an error; otherwise, it returns nil indicating a successful connection.
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

// PollRecords retrieves DNS records of a specified field type from the OVH API for the configured domain,
// filtering them to include only those whose target matches the provided public IP address.
// It returns a slice of recAndID structs containing details of the matching records and their IDs.
// If an error occurs during the API call, it returns nil and the error.
func PollRecords(client *ovh.Client, fieldType string, pubIP string) ([]recAndID, error) {
	var recordsIDs []int
	var records []recAndID
	err := client.Get(fmt.Sprintf("/domain/zone/%s/record?fieldType=%s", domain, fieldType), &recordsIDs)
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

// ManageRecords manages DNS records for a given field type and public IP address.
// It polls existing records and updates or creates them as necessary.
// If no records exist and there are no previous records, it creates a new record.
// If previous records exist but no current records match, it updates the previous records with the new public IP.
// After updating, it refreshes the DNS zone.
// Returns the updated list of records and any error encountered.
//
// Parameters:
//   client    - OVH API client used for DNS operations.
//   previous  - Slice of previous DNS records and their IDs.
//   fieldType - DNS record type (e.g., "A", "AAAA").
//   pubIP     - Public IP address to set in the DNS records.
//
// Returns:
//   []recAndID - Updated slice of DNS records and their IDs.
//   error      - Error encountered during the operation, if any.
func ManageRecords(client *ovh.Client, previous []recAndID, fieldType string, pubIP string) ([]recAndID, error) {
	records, err := PollRecords(client, fieldType, pubIP)
	if err != nil {
		log.Error().Err(err).Msgf("Failed to get %s records list", fieldType)
		return nil, err
	}

	if len(records) == 0 {
		if len(previous) == 0 {
			rec := NewRecord(fieldType, "", pubIP, 0)
			err = PostNewRecord(client, *rec)
			if err != nil {
				log.Error().Err(err).Str("IP", rec.Target).Int("TTL", rec.Ttl).Msg("Failed to add record")
				return nil, err
			} else {
				log.Info().Str("IP", rec.Target).Int("TTL", rec.Ttl).Msg("Sucessfully added record")
			}
		} else {
			for _, prev := range previous {
				rec := NewRecord(prev.FieldType, prev.Subdomain, pubIP, prev.Ttl)
				err := UpdateRecord(client, *rec, prev.Id)
				if err != nil {
					log.Error().Err(err).Str("type", prev.FieldType).Str("Subdomain", prev.Subdomain).Str("IP", prev.Target).Msg("Failed to update record")
				} else {
					log.Debug().Int("ID", prev.Id).Str("Type", prev.FieldType).Str("Subdomain", prev.Subdomain).Msg("Sucessfully updated record")
				}
			}
			err := RefreshZone(client)
			if err != nil {
				log.Error().Err(err).Msg("Failed to refresh DNS zone")
			}
			log.Info().Msgf("Updated %s records with new public IP [%s]", fieldType, pubIP)
		}
	} else {
		log.Info().Msgf("Public ip [%s] successfully found in %s record(s)", pubIP, fieldType)
		previous = records
	}
	return previous, nil
}

func main() {
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})

	parser := argparse.NewParser("govh-renew-ip", "Bite")
	logLevel := parser.Int("d", "debug", &argparse.Options{
		Required: false,
		Help:     "Select level logging, more info at https://github.com/rs/zerolog?tab=readme-ov-file#leveled-logging",
		Validate: func(args []string) error {
			level, err := strconv.Atoi(args[0])
			if err != nil || level < -1 || level > 5 {
				log.Fatal().Err(err).Msgf("Log level must be between -1 and 5, got %d", level)
			}
			return nil
		},
		Default: 1,
	})

	err := parser.Parse(os.Args)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to parse args")
	}

	zerolog.SetGlobalLevel(zerolog.Level(*logLevel))

	ctx, cancel := context.WithCancel(context.Background())

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigs
		log.Info().Str("signal", sig.String()).Msg("Received termination signal")
		cancel()
	}()

	domain, err = getEnv("DOMAIN")
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to get DOMAIN env variable")
	}

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

	var previousRecs = map[string][]recAndID{
		"A":    {},
		"AAAA": {},
	}

	for {
		select {
		case <-time.After(time.Duration(timeInterval) * time.Second):
			for _, fieldType := range []string{"A", "AAAA"} {
				// Poll public IP
				pubIP, err := getPublicIP(fieldType == "AAAA")
				if err != nil {
					log.Error().Err(err).Msgf("Failed to get public IP for type %s", fieldType)
					continue
				}
				// Manage records
				records, err := ManageRecords(client, previousRecs[fieldType], fieldType, pubIP)
				if err != nil {
					log.Error().Err(err).Msgf("Failed to manage %s records", fieldType)
					continue
				}
				previousRecs[fieldType] = records

			}
		case <-ctx.Done():
			log.Info().Msg("Closing program")
			return
		}
	}
}
