package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/go-telegram-bot-api/telegram-bot-api"
	"golang.org/x/time/rate"
)

type StatementItemData struct {
	Type string `json:"type"`
	Data struct {
		Account       string        `json:"account"`
		StatementItem StatementItem `json:"statementItem"`
	} `json:"data"`
}

type StatementItem struct {
	ID              string `json:"id"`
	Time            int    `json:"time"`
	Description     string `json:"description"`
	Comment         string `json:"comment"`
	Mcc             int    `json:"mcc"`
	Amount          int    `json:"amount"`
	OperationAmount int    `json:"operationAmount"`
	CurrencyCode    int    `json:"currencyCode"`
	CommissionRate  int    `json:"commissionRate"`
	CashbackAmount  int    `json:"cashbackAmount"`
	Balance         int    `json:"balance"`
	Hold            bool   `json:"hold"`
}

type ClientInfo struct {
	Name     string `json:"name"`
	Accounts []struct {
		ID           string `json:"id"`
		CurrencyCode int    `json:"currencyCode"`
		CashbackType string `json:"cashbackType"`
		Balance      int    `json:"balance"`
		CreditLimit  int    `json:"creditLimit"`
	} `json:"accounts"`
}

type IBot interface {
	TelegramStart()
	WebhookStart()
	ProcessingStart()
	SetWebHook(url string)
}

type bot struct {
	telegramToken  string
	telegramAdmins string
	telegramChats  string
	monoToken      string

	clientInfo ClientInfo

	BotAPI *tgbotapi.BotAPI

	monoLimiter *rate.Limiter
	ch          chan StatementItem
}

func NewBot(telegramToken, telegramAdmins, telegramChats, monoToken string) IBot {
	b := bot{
		telegramToken:  telegramToken,
		telegramAdmins: telegramAdmins,
		telegramChats:  telegramChats,
		monoToken:      monoToken,

		monoLimiter: rate.NewLimiter(rate.Every(time.Second*65), 1),
		ch:          make(chan StatementItem, 100),
	}

	return &b
}

func (b *bot) TelegramStart() {
	botAPI, err := tgbotapi.NewBotAPI(b.telegramToken)
	if err != nil {
		log.Panic(err)
	}

	b.BotAPI = botAPI

	//bot.Debug = true

	log.Printf("Authorized on account %s", b.BotAPI.Self.UserName)

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	updates, err := b.BotAPI.GetUpdatesChan(u)

	for update := range updates {
		if update.Message == nil || !(b.isAdmin(update.Message.From.ID) || b.isChat(update.Message.Chat.ID)) {
			continue
		}

		log.Printf("[telegram] received a message from %d", update.Message.From.ID)
		log.Printf("[telegram] received a message in chat %d", update.Message.Chat.ID)

		if update.Message.Text == "/balance" {
			if b.monoLimiter.Allow() {
				log.Printf("[monoapi] Fetching...")
				b.clientInfo = b.getClientInfo()
			}

			var balance int
			for _, account := range b.clientInfo.Accounts {
				if account.CurrencyCode == 980 {
					balance = account.Balance
				}
			}

			message := fmt.Sprintf("Баланс %.2f₴", float64(balance/100))
			msg := tgbotapi.NewMessage(update.Message.Chat.ID, message)
			msg.ReplyToMessageID = update.Message.MessageID

			b.BotAPI.Send(msg)
		}
	}
}

func (b *bot) WebhookStart() {
	http.HandleFunc("/web_hook", func(w http.ResponseWriter, r *http.Request) {
		body, err := ioutil.ReadAll(r.Body)
		if err != nil {
			log.Printf("[monoapi webhook] error %s", err)

			fmt.Fprintf(w, "Not Ok!")
			return
		}

		//log.Printf("[monoapi webhook] body %s", string(body))

		var statementItemData StatementItemData
		if err := json.Unmarshal(body, &statementItemData); err != nil {
			log.Printf("[monoapi webhook] unmarshal error %s", err)

			fmt.Fprintf(w, "Not Ok!")
			return
		}

		b.ch <- statementItemData.Data.StatementItem

		fmt.Fprintf(w, "Ok!")
	})

	err := http.ListenAndServe(":8080", nil)
	if err != nil {
		log.Panic(err)
	}
}

func (b *bot) ProcessingStart() {
	for {
		select {
		case statementItem := <-b.ch:
			message := fmt.Sprintf(
				"Страченно %s на %s, %s \nCashback: %s\nБаланс %s",
				b.normalizePrice(statementItem.Amount),
				statementItem.Description,
				statementItem.Comment,
				b.normalizePrice(statementItem.CashbackAmount),
				b.normalizePrice(statementItem.Balance),
			)

			// to chat
			ids := strings.Split(strings.Trim(b.telegramChats, " "), ",")
			for _, id := range ids {
				chatID, err := strconv.ParseInt(id, 10, 64)
				if err != nil {
					log.Printf("[processing] parseInt error %s", err)
				}

				msg := tgbotapi.NewMessage(chatID, message)
				_, err = b.BotAPI.Send(msg)
				if err != nil {
					log.Printf("[processing] send message error %s", err)
				}
			}

			// to admin member
			ids = strings.Split(strings.Trim(b.telegramAdmins, " "), ",")
			for _, id := range ids {
				chatID, err := strconv.ParseInt(id, 10, 64)
				if err != nil {
					log.Printf("[processing] parseInt error %s", err)
				}

				msg := tgbotapi.NewMessage(chatID, message)
				_, err = b.BotAPI.Send(msg)
				if err != nil {
					log.Printf("[processing] send message error %s", err)
				}
			}
		}
	}
}

func (b bot) SetWebHook(url string) {
	payload := strings.NewReader(fmt.Sprintf("{\"webHookUrl\": \"%s\"}", url))

	req, _ := http.NewRequest("POST", "https://api.monobank.ua/personal/webhook", payload)

	req.Header.Add("X-Token", b.monoToken)
	req.Header.Add("content-type", "application/json")

	res, _ := http.DefaultClient.Do(req)

	defer res.Body.Close()
	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		log.Printf("[set webHook] error %s", err)
	}

	log.Printf("[set webHook] responce %s", string(body))
}

func (b bot) getClientInfo() ClientInfo {
	url := "https://api.monobank.ua/personal/client-info"
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		log.Printf("[monoapi] create request error %s", err)
	}

	req.Header.Add("x-token", b.monoToken)

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Printf("[monoapi] request error %s", err)
	}

	defer res.Body.Close()
	body, _ := ioutil.ReadAll(res.Body)

	var clientInfo ClientInfo
	if err := json.Unmarshal(body, &clientInfo); err != nil {
		log.Printf("[monoapi] unmarshal error %s", err)
	}

	return clientInfo
}

func (b bot) isAdmin(userId int) bool {
	ids := strings.Split(strings.Trim(b.telegramAdmins, " "), ",")
	for _, id := range ids {
		if reflect.DeepEqual([]byte(id), []byte(strconv.Itoa(userId))) {
			return true
		}
	}

	return false
}

func (b bot) isChat(chatId int64) bool {
	ids := strings.Split(strings.Trim(b.telegramChats, " "), ",")
	for _, id := range ids {
		if reflect.DeepEqual([]byte(id), []byte(strconv.FormatInt(chatId, 10))) {
			return true
		}
	}

	return false
}

func (b bot) normalizePrice(price int) string {
	return fmt.Sprintf("%.2f₴", float64(price/100))
}
