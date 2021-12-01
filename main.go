package main

import (
	"encoding/json"
	"fmt"
	"github.com/caarlos0/env/v6"
	"github.com/twilio/twilio-go"
	"net/http"
	"net/url"
	"time"

	log "github.com/sirupsen/logrus"
	openapi "github.com/twilio/twilio-go/rest/api/v2010"
)

type Config struct {
	TwilioSid            string `env:"TWILIO_ACCOUNT_SID,notEmpty"`
	TwilioAuth           string `env:"TWILIO_AUTH_TOKEN,notEmpty"`
	TwilioDest           string `env:"TWILIO_DEST,notEmpty"`
	TwilioFrom           string `env:"TWILIO_FROM,notEmpty"`
	LogLevel             string `env:"LOG_LEVEL" envDefault:"INFO"`
	YelpQueryUrl         string `env:"YELP_QUERY_URL,notEmpty"`
	YelpQueryDateOffset  int    `env:"YELP_QUERY_DATE_OFFSET" envDefault:"60"`
	YelpQueryDaysAfter   string `env:"YELP_QUERY_DAYS_AFTER" envDefault:"7"`
	YelpQueryDaysBefore  string `env:"YELP_QUERY_DAYS_BEFORE" envDefault:"7"`
	YelpQueryPartySize   string `env:"YELP_QUERY_PARTY_SIZE" envDefault:"2"`
	QueryIntervalSeconds int    `env:"QUERY_INTERVAL_SECONDS" envDefault:"60"`
	RequestTimeout       int    `env:"TIMEOUT_SECONDS" envDefault:"30"`
	ProxyUrl             string `env:"PROXY_URL"`
}

type AvailabilityResult struct {
	Success          bool `json:"success"`
	AvailabilityData []struct {
		AvailabilityList []struct {
			Timestamp     int    `json:"timestamp"`
			FormattedTime string `json:"formatted_time"`
			FormAction    string `json:"form_action"`
			CsrfToken     string `json:"csrf_token"`
			Isodate       string `json:"isodate"`
		} `json:"availability_list"`
		Date      string `json:"date"`
		Covers    int    `json:"covers"`
		Time      string `json:"time"`
		Timestamp int    `json:"timestamp"`
		Msg       string `json:"msg"`
		Isodate   string `json:"isodate"`
	} `json:"availability_data"`
	AvailabilityProfile interface{} `json:"availability_profile"`
	ExactMatch          struct {
		Timestamp     int    `json:"timestamp"`
		FormattedTime string `json:"formatted_time"`
		FormAction    string `json:"form_action"`
		CsrfToken     string `json:"csrf_token"`
		Isodate       string `json:"isodate"`
	} `json:"exact_match"`
	NotifyMeMessage     interface{} `json:"notify_me_message"`
	NotifyMeURL         interface{} `json:"notify_me_url"`
	EnableNextAvailable bool        `json:"enable_next_available"`
	MotivationalContent interface{} `json:"motivational_content"`
	RecoveryProfile     interface{} `json:"recovery_profile"`
}

var cfg Config

//Get formatted date string based on configurable positive offset
func getDateString() string {
	now := time.Now()
	queryDate := now.AddDate(0, 0, cfg.YelpQueryDateOffset)
	return queryDate.Format("2006-01-02")
}

//Build query/URL and send request to Yelp
func sendRequest() (AvailabilityResult, error) {
	var results AvailabilityResult

	u, err := url.Parse(cfg.YelpQueryUrl)
	if err != nil {
		log.WithFields(log.Fields{"YelpQueryUrl": cfg.YelpQueryUrl}).Fatal("unable to parse yelp query URL")
	}
	//Set query parameters for yelp query
	params := u.Query()
	params.Set("date", getDateString())
	params.Set("days_before", cfg.YelpQueryDaysBefore)
	params.Set("days_after", cfg.YelpQueryDaysAfter)
	params.Set("covers", cfg.YelpQueryPartySize)
	client := &http.Client{}
	//Use proxy if configured
	if cfg.ProxyUrl != "" {
		proxyUrl, err := url.Parse(cfg.ProxyUrl)
		if err != nil {
			log.WithFields(log.Fields{"proxyUrl": cfg.ProxyUrl}).Error("unable to parse proxyurl")
		} else {
			transport := &http.Transport{
				Proxy: http.ProxyURL(proxyUrl),
			}
			client.Transport = transport
		}
	}
	client.Timeout = time.Duration(cfg.RequestTimeout) * time.Second

	req, err := http.NewRequest("GET", u.String(), nil)
	if err != nil {
		return results, err
	}
	req.URL.RawQuery = params.Encode()
	req.Header.Set("authority", "www.yelp.com")
	req.Header.Set("accept", "application/json, text/plain, */*")
	req.Header.Set("x-requested-with", "XMLHttpRequest")
	resp, err := client.Do(req)
	if err != nil {
		return results, err
	}
	err = json.NewDecoder(resp.Body).Decode(&results)
	if err != nil {
		return results, err
	}
	return results, nil
}

func sendSms(message string) {
	client := twilio.NewRestClientWithParams(twilio.RestClientParams{
		Username: cfg.TwilioSid,
		Password: cfg.TwilioAuth,
	})
	params := &openapi.CreateMessageParams{}
	params.SetTo(cfg.TwilioDest)
	params.SetFrom(cfg.TwilioFrom)
	params.SetBody(message)
	resp, err := client.ApiV2010.CreateMessage(params)
	if err != nil {
		log.Error(err)
		err = nil
	} else {
		log.WithFields(log.Fields{"sid": resp.Sid}).Debug("message sent to twilio")
	}
}

func parseResults(result AvailabilityResult) error {
	if len(result.AvailabilityData) == 0 {
		log.Warn("no results found")
	} else {
		log.Info("parsing reservation results")
		for _, s := range result.AvailabilityData {
			if len(s.AvailabilityList) == 0 {
				log.WithFields(log.Fields{"date": s.Date}).Debug("no available reservations")
			} else {
				for _, r := range s.AvailabilityList {
					log.WithFields(log.Fields{"result": r}).Debug("available reservation found")
					var alertString = fmt.Sprintf("Reservation Found for %s. https://www.yelp.com%s", r.Isodate, r.FormAction)
					sendSms(alertString)
				}
			}
		}
	}
	return nil
}

func findReservations() error {
	log.Info("searching for available reservations...")
	result, err := sendRequest()
	if err != nil {
		return err
	}
	err = parseResults(result)
	if err != nil {
		return err
	}
	return nil
}

func init() {
	if err := env.Parse(&cfg); err != nil {
		log.Fatal(err)
	}
	level, err := log.ParseLevel(cfg.LogLevel)
	if err != nil {
		log.WithFields(log.Fields{"LOG_LEVEL": cfg.LogLevel}).Warn("unable to parse LOG_LEVEL")
	} else {
		log.SetLevel(level)
	}
	log.SetFormatter(&log.TextFormatter{
		FullTimestamp:   true,
		TimestampFormat: "2006-01-02 15:04:05",
		DisableColors:   false,
	})
	//log.SetFormatter(&log.JSONFormatter{})
}

func main() {
	log.Info("starting app")
	for {
		err := findReservations()
		if err != nil {
			log.Error(err)
		}
		time.Sleep(time.Duration(cfg.QueryIntervalSeconds) * time.Second)
	}
}
