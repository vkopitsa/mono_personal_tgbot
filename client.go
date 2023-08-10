package main

import (
	"errors"
	"fmt"
	"hash/fnv"

	"github.com/rs/zerolog/log"
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
	Clear() Client

	ResetReport(accountId string)
	GetAccountByID(id string) (*Account, error)
}

type client struct {
	Info    *ClientInfo
	id      uint32
	token   string
	reports map[string]Report
	mono    *Mono
}

// NewClient returns a client object.
func NewClient(token string, mono *Mono) Client {

	h := fnv.New32a()
	h.Write([]byte(token))

	return &client{
		token:   token,
		id:      h.Sum32(),
		reports: make(map[string]Report),
		mono:    mono,
	}
}

func (c *client) Init() error {
	info, err := c.GetInfo()
	c.Info = &info
	return err
}

func (c client) GetID() uint32 {
	return c.id
}

func (c *client) GetReport(accountId string) Report {
	if _, ok := c.reports[accountId]; !ok {
		account, err := c.GetAccountByID(accountId)
		if err != nil {
			return nil
		}
		c.reports[accountId] = NewReport(account, c.id)
	}

	return c.reports[accountId]
}

func (c *client) GetInfo() (ClientInfo, error) {
	if c.Info != nil {
		return *c.Info, nil
	}

	log.Debug().Msg("[monoapi] get info")
	info, err := c.mono.GetClientInfo(c.token)
	c.Info = &info
	return *c.Info, err
}

// GetName return name of the client
func (c client) GetName() string {
	if c.Info == nil {
		return "NoName"
	}
	return c.Info.Name
}

// Clear clear vars of the client
func (c *client) Clear() Client {
	c.Info = nil

	return c
}

// SetWebHook is a function set up the monobank webhook.
func (c client) SetWebHook(url string) (WebHookResponse, error) {
	return c.mono.SetWebHook(url, c.token)
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
	return c.mono.GetStatement(command, accountId, c.token)
}

func (c Account) GetName() string {
	return fmt.Sprintf("%s %s", c.Type, GetCurrencySymbol(c.CurrencyCode))
}
