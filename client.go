package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"io/ioutil"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/looplab/fsm"
	"golang.org/x/time/rate"
)

// StatementItem is a statement data
type StatementItem struct {
	ID              string `json:"id"`
	Time            int    `json:"time"`
	Description     string `json:"description"`
	Comment         string `json:"comment,omitempty"`
	Mcc             int    `json:"mcc"`
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
	ID           string `json:"id"`
	CurrencyCode int    `json:"currencyCode"`
	CashbackType string `json:"cashbackType"`
	Balance      int    `json:"balance"`
	CreditLimit  int    `json:"creditLimit"`
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
	GetReport() Report
	GetInfo() (ClientInfo, error)
	GetStatement(command string) ([]StatementItem, error)
	SetWebHook(url string) (WebHookResponse, error)
	GetName() string

	IsState(flag ClientState) bool
	Can(flag ClientState) bool
	SetState(flag ClientState)
	AddStatementItem(string, StatementItem)
}

// ClientState is a state type
type ClientState uint

const (
	// ClientStateNone is a none state
	ClientStateNone ClientState = iota
	// ClientStateReport is a report state
	ClientStateReport
	// ClientStateWebHook is a webhook state
	ClientStateWebHook
)

type client struct {
	Info    *ClientInfo
	id      uint32
	token   string
	limiter *rate.Limiter
	report  Report
	//state   ClientState
	fsm *fsm.FSM
}

// NewClient returns a client object.
func NewClient(token string) Client {

	h := fnv.New32a()
	h.Write([]byte(token))

	return &client{
		limiter: rate.NewLimiter(rate.Every(time.Second*65), 1),
		token:   token,
		id:      h.Sum32(),
		report:  NewReport(),
		fsm: fsm.NewFSM(
			"none",
			fsm.Events{
				{Name: "none", Src: []string{"Report", "WebHook"}, Dst: "none"},
				{Name: "Report", Src: []string{"none"}, Dst: "Report"},
				{Name: "WebHook", Src: []string{"none"}, Dst: "WebHook"},
			},
			fsm.Callbacks{},
		),
	}
}

func (c *client) Init() error {
	_, err := c.GetInfo()
	return err
}

func (c client) GetID() uint32 {
	return c.id
}

func (c *client) GetReport() Report {
	return c.report
}

func (c *client) GetInfo() (ClientInfo, error) {
	if c.limiter.Allow() {
		log.Printf("[monoapi] get info")
		info, err := c.getClientInfo()
		c.Info = &info
		return info, err
	}

	if c.Info != nil {
		return *c.Info, nil
	}

	log.Printf("[monoapi] get info, waiting 1 minute")
	return ClientInfo{}, errors.New("please waiting 1 minute and then try again")
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
		log.Printf("[monoapi] webhook, NewRequest %s", err)
		return response, err
	}

	req.Header.Add("X-Token", c.token)
	req.Header.Add("content-type", "application/json")

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Printf("[monoapi] webhook, error %s", err)
		return response, err
	}

	defer res.Body.Close()
	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		log.Printf("[monoapi] webhook, error %s", err)
		return response, err
	}

	if err := json.Unmarshal(body, &response); err != nil {
		log.Printf("[monoapi] webhook, unmarshal error %s", err)
		return response, err
	}

	log.Printf("[monoapi] webhook, responce %s", string(body))
	return response, err
}

func (c *client) AddStatementItem(account string, statementItem StatementItem) {
	c.GetReport().ResetLastData()
}

func (c client) GetStatement(command string) ([]StatementItem, error) {
	if c.limiter.Allow() {
		return c.getStatement(command)
	}

	log.Printf("[monoapi] statement, waiting 1 minute")
	return []StatementItem{}, errors.New("please waiting 1 minute and then try again")
}

func (c client) getStatement(command string) ([]StatementItem, error) {

	statementItems := []StatementItem{}

	from, to, err := getTimeRangeByPeriod(command)
	if err != nil {
		log.Printf("[monoapi] statements, range error %s", err)
		return statementItems, err
	}

	log.Printf("[monoapi] statements, range from: %d, to: %d", from, to)

	url := fmt.Sprintf("https://api.monobank.ua/personal/statement/0/%d", from)
	if to > 0 {
		url = fmt.Sprintf("%s/%d", url, to)
	}

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		log.Printf("[monoapi] statements, NewRequest error %s", err)
		return statementItems, err
	}

	req.Header.Add("x-token", c.token)

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Printf("[monoapi] statements, error %s", err)
		return statementItems, err
	}

	defer res.Body.Close()
	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		log.Printf("[monoapi] statements, error %s", err)
		return statementItems, err
	}

	//log.Printf("[monoapi] statements, body %s", string(body))

	if err := json.Unmarshal(body, &statementItems); err != nil {
		log.Printf("[monoapi] statements, unmarshal error %s", err)
		return statementItems, err
	}

	return statementItems, nil
}

func (c client) getClientInfo() (ClientInfo, error) {
	var clientInfo ClientInfo

	url := "https://api.monobank.ua/personal/client-info"
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		log.Printf("[monoapi] client info, create request error %s", err)
		return clientInfo, err
	}

	req.Header.Add("x-token", c.token)

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Printf("[monoapi] client info, request error %s", err)
		return clientInfo, err
	}

	defer res.Body.Close()
	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return clientInfo, err
	}

	if err := json.Unmarshal(body, &clientInfo); err != nil {
		log.Printf("[monoapi] client info, unmarshal error %s", err)
		return clientInfo, err
	}

	return clientInfo, nil
}

func (c *client) SetState(state ClientState) {
	if !c.IsState(state) {
		c.fsm.Event(c.getStringFromState(state))
	}
}

func (c client) IsState(state ClientState) bool {
	return c.fsm.Is(c.getStringFromState(state))
}

func (c client) Can(state ClientState) bool {
	return c.fsm.Can(c.getStringFromState(state))
}

func (c *client) getStringFromState(state ClientState) string {
	switch state {
	case ClientStateReport:
		return "Report"
	case ClientStateWebHook:
		return "WebHook"
	default:
		return "none"
	}
}

func (c *client) getFlagFromString(state string) ClientState {
	switch state {
	case "Report":
		return ClientStateReport
	case "WebHook":
		return ClientStateWebHook
	default:
		return ClientStateNone
	}
}
