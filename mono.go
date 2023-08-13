package main

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/rs/zerolog/log"
	"golang.org/x/time/rate"
)

type Currency struct {
	CurrencyCodeA int     `json:"currencyCodeA"`
	CurrencyCodeB int     `json:"currencyCodeB"`
	Date          int     `json:"date"`
	RateBuy       float64 `json:"rateBuy"`
	RateCross     float64 `json:"rateCross"`
	RateSell      float64 `json:"rateSell"`
}

type Currencies []Currency

type Mono struct {
	limiter  *rate.Limiter
	limiter2 *rate.Limiter
	limiter3 *rate.Limiter
	limiter4 *rate.Limiter
}

// NewMono returns a mono object.
func NewMono() *Mono {
	return &Mono{
		limiter:  rate.NewLimiter(rate.Every(time.Second*60), 1),
		limiter2: rate.NewLimiter(rate.Every(time.Second*60), 1),
		limiter3: rate.NewLimiter(rate.Every(time.Second*60), 1),
		limiter4: rate.NewLimiter(rate.Every(time.Second*60), 1),
	}
}

// SetWebHook is a function set up the monobank webhook.
func (c Mono) SetWebHook(url, token string) (WebHookResponse, error) {
	if !c.limiter.Allow() {
		time.Sleep(61 * time.Second)
	}

	response := WebHookResponse{}

	payload := strings.NewReader(fmt.Sprintf("{\"webHookUrl\": \"%s\"}", url))

	req, err := http.NewRequest("POST", "https://api.monobank.ua/personal/webhook", payload)
	if err != nil {
		log.Error().Err(err).Msg("[monoapi] webhook, NewRequest")
		return response, err
	}

	req.Header.Add("X-Token", token)
	req.Header.Add("content-type", "application/json")

	return DoRequest(response, req)
}

func (c Mono) GetStatement(command, account string, token string) ([]StatementItem, error) {
	if !c.limiter2.Allow() {
		time.Sleep(61 * time.Second)
	}

	statementItems := []StatementItem{}

	from, to, err := getTimeRangeByPeriod(command)
	if err != nil {
		log.Error().Err(err).Msg("[monoapi] statements, range")
		return statementItems, err
	}

	log.Debug().Msgf("[monoapi] statements, range from: %d, to: %d", from, to)

	url := fmt.Sprintf("https://api.monobank.ua/personal/statement/%s/%d", account, from)
	if to > 0 {
		url = fmt.Sprintf("%s/%d", url, to)
	}

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		log.Error().Err(err).Msg("[monoapi] statements, NewRequest")
		return statementItems, err
	}

	req.Header.Add("x-token", token)

	return DoRequest(statementItems, req)
}

func (c Mono) GetClientInfo(token string) (ClientInfo, error) {
	if !c.limiter3.Allow() {
		time.Sleep(61 * time.Second)
	}

	var clientInfo ClientInfo

	url := "https://api.monobank.ua/personal/client-info"
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		log.Error().Err(err).Msg("[monoapi] client info, create request")
		return clientInfo, err
	}

	req.Header.Add("x-token", token)

	return DoRequest(clientInfo, req)
}

func (c Mono) GetCurrencies() (Currencies, error) {
	if !c.limiter4.Allow() {
		time.Sleep(61 * time.Second)
	}

	var currencies Currencies

	url := "https://api.monobank.ua/bank/currency"
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		log.Error().Err(err).Msg("[monoapi] currency")
		return currencies, err
	}

	return DoRequest(currencies, req)
}
