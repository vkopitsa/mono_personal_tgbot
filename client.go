package main

import (
	"errors"
	"fmt"
	"hash/fnv"
	"net/http"
	"strings"
	"time"

	"github.com/rs/zerolog/log"

	"golang.org/x/time/rate"
)

// StatementItem is a statement data
type StatementItem struct {
	ID              string `json:"id"`
	Time            int    `json:"time"`
	Description     string `json:"description"`
	Comment         string `json:"comment,omitempty"`
	Mcc             int    `json:"mcc"`
	OriginalMcc     int    `json:"originalMcc"`
	Amount          int    `json:"amount"`
	OperationAmount int    `json:"operationAmount"`
	CurrencyCode    int    `json:"currencyCode"`
	CommissionRate  int    `json:"commissionRate"`
	CashbackAmount  int    `json:"cashbackAmount"`
	Balance         int    `json:"balance"`
	Hold            bool   `json:"hold"`
}

// Account is a account information
type Account struct {
	ID           string   `json:"id"`
	Type         string   `json:"type"`
	CurrencyCode int      `json:"currencyCode"`
	CashbackType string   `json:"cashbackType"`
	Balance      int      `json:"balance"`
	CreditLimit  int      `json:"creditLimit"`
	Iban         string   `json:"iban"`
	MaskedPan    []string `json:"maskedPan"`
}

// ClientInfo is a client information
type ClientInfo struct {
	Name       string    `json:"name"`
	WebHookURL string    `json:"webHookUrl,omitempty"`
	Accounts   []Account `json:"accounts"`
}

// WebHookResponse is a response from api on setup webhook
type WebHookResponse struct {
	ErrorDescription string `json:"errorDescription"`
	Status           string `json:"status"`
}

// Client is the interface representing client object.
type Client interface {
	Init() error
	GetID() uint32
	GetReport(accountId string) Report
	GetInfo() (ClientInfo, error)
	GetStatement(command, accountId string) ([]StatementItem, error)
	SetWebHook(url string) (WebHookResponse, error)
	GetName() string

	ResetReport(accountId string)
	GetAccountByID(id string) (*Account, error)
}

type client struct {
	Info    *ClientInfo
	id      uint32
	token   string
	limiter *rate.Limiter
	reports map[string]Report
}

// NewClient returns a client object.
func NewClient(token string) Client {

	h := fnv.New32a()
	h.Write([]byte(token))

	return &client{
		limiter: rate.NewLimiter(rate.Every(time.Second*30), 1),
		token:   token,
		id:      h.Sum32(),
		reports: make(map[string]Report),
	}
}

func (c *client) Init() error {
	_, err := c.GetInfo()
	return err
}

func (c client) GetID() uint32 {
	return c.id
}

func (c *client) GetReport(accountId string) Report {
	if _, ok := c.reports[accountId]; !ok {
		c.reports[accountId] = NewReport(accountId, c.id)
	}

	return c.reports[accountId]
}

func (c *client) GetInfo() (ClientInfo, error) {
	if c.limiter.Allow() {
		log.Debug().Msg("[monoapi] get info")
		info, err := c.getClientInfo()
		c.Info = &info
		return info, err
	}

	if c.Info != nil {
		return *c.Info, nil
	}

	log.Warn().Msg("[monoapi] get info, waiting")
	return ClientInfo{}, errors.New("please waiting and then try again")
}

// GetName return name of the client
func (c client) GetName() string {
	if c.Info == nil {
		return "NoName"
	}
	return c.Info.Name
}

// SetWebHook is a function set up the monobank webhook.
func (c client) SetWebHook(url string) (WebHookResponse, error) {
	response := WebHookResponse{}

	payload := strings.NewReader(fmt.Sprintf("{\"webHookUrl\": \"%s\"}", url))

	req, err := http.NewRequest("POST", "https://api.monobank.ua/personal/webhook", payload)
	if err != nil {
		log.Error().Err(err).Msg("[monoapi] webhook, NewRequest")
		return response, err
	}

	req.Header.Add("X-Token", c.token)
	req.Header.Add("content-type", "application/json")

	return DoRequest(response, req)
}

func (c *client) GetAccountByID(id string) (*Account, error) {
	if c.Info != nil {
		for _, account := range c.Info.Accounts {
			if account.ID == id {
				return &account, nil
			}
		}
	}

	return nil, errors.New("account does not found")
}

func (c *client) ResetReport(accountId string) {
	c.GetReport(accountId).ResetLastData()
}

func (c client) GetStatement(command string, accountId string) ([]StatementItem, error) {
	if c.limiter.Allow() {
		return c.getStatement(command, accountId)
	}

	log.Warn().Msg("[monoapi] statement, waiting")
	return []StatementItem{}, errors.New("please waiting and then try again")
}

func (c client) getStatement(command, account string) ([]StatementItem, error) {

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

	req.Header.Add("x-token", c.token)

	return DoRequest(statementItems, req)
}

func (c client) getClientInfo() (ClientInfo, error) {
	var clientInfo ClientInfo

	url := "https://api.monobank.ua/personal/client-info"
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		log.Error().Err(err).Msg("[monoapi] client info, create request")
		return clientInfo, err
	}

	req.Header.Add("x-token", c.token)

	return DoRequest(clientInfo, req)
}
