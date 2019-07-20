package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
	"io/ioutil"
	"log"
	"net/http"
	"reflect"
	"strconv"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api"
	"golang.org/x/time/rate"
)

// StatementItemData is a response from webhook with statement
type StatementItemData struct {
	Type string `json:"type"`
	Data struct {
		Account       string        `json:"account"`
		StatementItem StatementItem `json:"statementItem"`
	} `json:"data"`
}

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
	Name     string    `json:"name"`
	Accounts []Account `json:"accounts"`
}

// Bot is the interface representing bot object.
type Bot interface {
	TelegramStart()
	WebhookStart()
	ProcessingStart()
	SetWebHook(url string) ([]byte, error)
}

// bot is implementation the Bot interface
type bot struct {
	telegramToken  string
	telegramAdmins string
	telegramChats  string
	monoToken      string

	clientInfo ClientInfo

	BotAPI *tgbotapi.BotAPI

	monoLimiter *rate.Limiter
	ch          chan StatementItem

	statementTmpl *template.Template
	balanceTmpl   *template.Template

	report Report
}

// New returns a bot object.
func New(telegramToken, telegramAdmins, telegramChats, monoToken string) Bot {

	statementTmpl, err := GetTempate(statementTemplate)
	if err != nil {
		log.Fatalf("[template] %s", err)
	}

	balanceTmpl, err := GetTempate(balanceTemplate)
	if err != nil {
		log.Fatalf("[template] %s", err)
	}

	b := bot{
		telegramToken:  telegramToken,
		telegramAdmins: telegramAdmins,
		telegramChats:  telegramChats,
		monoToken:      monoToken,

		monoLimiter: rate.NewLimiter(rate.Every(time.Second*65), 1),
		ch:          make(chan StatementItem, 100),

		statementTmpl: statementTmpl,
		balanceTmpl:   balanceTmpl,

		report: NewReport(),
	}

	return &b
}

// TelegramStart starts getting updates from telegram.
func (b *bot) TelegramStart() {
	botAPI, err := tgbotapi.NewBotAPI(b.telegramToken)
	if err != nil {
		log.Panic("[telegram] create bot ", err)
	}

	b.BotAPI = botAPI

	//bot.Debug = true

	log.Printf("Authorized on account %s", b.BotAPI.Self.UserName)

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	updates, err := b.BotAPI.GetUpdatesChan(u)

	for update := range updates {

		log.Printf("[telegram] dfsdf")

		if update.Message != nil {
			log.Printf("[telegram] received a message from %d in chat %d",
				update.Message.From.ID, update.Message.Chat.ID)
		}

		if update.Message != nil && update.Message.Text == "/balance" {
			if b.monoLimiter.Allow() {
				log.Printf("[monoapi] Fetching...")
				b.clientInfo, err = b.getClientInfo()
				if err != nil {
					continue
				}
			} else {
				log.Printf("[telegram] balance, waiting 1 minute")
			}

			var account Account
			for _, _account := range b.clientInfo.Accounts {
				if _account.CurrencyCode == 980 {
					account = _account
				}
			}

			var tpl bytes.Buffer
			err := b.balanceTmpl.Execute(&tpl, account)
			if err != nil {
				log.Printf("[telegram] balance, template execute error %s", err)
				continue
			}
			message := tpl.String()

			msg := tgbotapi.NewMessage(update.Message.Chat.ID, message)
			msg.ReplyToMessageID = update.Message.MessageID

			_, err = b.BotAPI.Send(msg)
			if err != nil {
				log.Printf("[telegram] balance, send msg error %s", err)
			}
		} else if update.Message != nil && update.Message.Text == "/report" {
			log.Printf("[telegram] report show keyboard")

			_, err := b.BotAPI.Send(b.report.GetKeyboardMessageConfig(update))
			if err != nil {
				log.Printf("[telegram] report send msg error %s", err)
			}
		} else if update.Message != nil && b.report.IsReportGridCommand(update) {
			log.Printf("[telegram] report grid")

			if !b.monoLimiter.Allow() {
				log.Printf("[telegram] report grid, waiting 1 minute")

				msg := tgbotapi.NewMessage(update.Message.Chat.ID, "Please waiting 1 minute and then try again.")
				_, err = b.BotAPI.Send(msg)

				continue
			}

			items, err := b.getStatements(b.report.GetPeriodFromUpdate(update))
			if err != nil {
				log.Printf("[telegram] report get statements error %s", err)
				continue
			}

			// init statements data
			b.report.SetGridData(update, items)

			_, err = b.BotAPI.Send(b.report.GetReportGrid(update))
			if err != nil {
				log.Printf("[telegram] report grid send error %s", err)
			}
		} else if update.CallbackQuery != nil && b.report.IsReportGridPageCommand(update) {
			log.Printf("[telegram] report grid page")

			if !b.report.IsExistGridData(update) {

				if !b.monoLimiter.Allow() {
					log.Printf("[telegram] report grid page, waiting 1 minute")

					msg := tgbotapi.NewMessage(update.CallbackQuery.Message.Chat.ID, "Please waiting 1 minute and then try again.")
					_, err = b.BotAPI.Send(msg)

					continue
				}

				items, err := b.getStatements(b.report.GetPeriodFromUpdate(update))
				if err != nil {
					log.Printf("[telegram] report grid page get statements error %s", err)
					continue
				}

				// reinit statements data if does not exist
				b.report.SetGridData(update, items)
			}

			editMessage, err := b.report.GetUpdatedReportGrid(update)
			if err != nil {
				_, err = b.BotAPI.AnswerCallbackQuery(tgbotapi.CallbackConfig{
					CallbackQueryID: update.CallbackQuery.ID,
					Text:            "Error :(",
				})
				if err != nil {
					log.Printf("[telegram] report grid send callback answer on update error %s", err)
				}
			}

			_, err = b.BotAPI.Send(editMessage)
			if err != nil {
				log.Printf("[telegram] report grid send error %s", err)
			}

			_, err = b.BotAPI.AnswerCallbackQuery(tgbotapi.CallbackConfig{
				CallbackQueryID: update.CallbackQuery.ID,
			})
			if err != nil {
				log.Printf("[telegram] report grid send callback answer error %s", err)
			}
		}
	}
}

// TelegramStart starts web server for getting webhooks from the monobank.
// It run a http handle and a received StatementItemData data sent to the channel for processing.
func (b *bot) WebhookStart() {
	http.HandleFunc("/web_hook", func(w http.ResponseWriter, r *http.Request) {
		body, err := ioutil.ReadAll(r.Body)
		if err != nil {
			log.Printf("[webhook] error %s", err)

			fmt.Fprintf(w, "Not Ok!")
			return
		}

		//log.Printf("[webhook] body %s", string(body))

		var statementItemData StatementItemData
		if err := json.Unmarshal(body, &statementItemData); err != nil {
			log.Printf("[webhook] unmarshal error %s", err)

			fmt.Fprintf(w, "Not Ok!")
			return
		}

		b.ch <- statementItemData.Data.StatementItem

		fmt.Fprintf(w, "Ok!")
	})

	err := http.ListenAndServe(":8080", nil)
	if err != nil {
		log.Panic("[webhook] serve ", err)
	}
}

// ProcessingStart starts processing data that received from chennal.
func (b *bot) ProcessingStart() {
	for {
		select {
		case statementItem := <-b.ch:
			var tpl bytes.Buffer
			err := b.statementTmpl.Execute(&tpl, statementItem)
			if err != nil {
				log.Printf("[processing] template execute error %s", err)
				continue
			}
			message := tpl.String()

			// to chat
			ids := strings.Split(strings.Trim(b.telegramChats, " "), ",")
			for _, id := range ids {
				chatID, err := strconv.ParseInt(id, 10, 64)
				if err != nil {
					log.Printf("[processing] parseInt error %s", err)
					continue
				}

				msg := tgbotapi.NewMessage(chatID, message)
				_, err = b.BotAPI.Send(msg)
				if err != nil {
					log.Printf("[processing] send message error %s", err)
					continue
				}
			}

			// to admin member
			ids = strings.Split(strings.Trim(b.telegramAdmins, " "), ",")
			for _, id := range ids {
				chatID, err := strconv.ParseInt(id, 10, 64)
				if err != nil {
					log.Printf("[processing] parseInt error %s", err)
					continue
				}

				msg := tgbotapi.NewMessage(chatID, message)
				_, err = b.BotAPI.Send(msg)
				if err != nil {
					log.Printf("[processing] send message error %s", err)
					continue
				}
			}
		}
	}
}

// SetWebHook is a function set up the monobank webhook.
func (b bot) SetWebHook(url string) ([]byte, error) {
	payload := strings.NewReader(fmt.Sprintf("{\"webHookUrl\": \"%s\"}", url))

	req, err := http.NewRequest("POST", "https://api.monobank.ua/personal/webhook", payload)
	if err != nil {
		log.Printf("[monoapi] webhook, NewRequest %s", err)
		return []byte{}, err
	}

	req.Header.Add("X-Token", b.monoToken)
	req.Header.Add("content-type", "application/json")

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Printf("[monoapi] webhook, error %s", err)
		return []byte{}, err
	}

	defer res.Body.Close()
	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		log.Printf("[monoapi] webhook, error %s", err)
		return []byte{}, err
	}

	log.Printf("[monoapi] webhook, responce %s", string(body))
	return body, err
}

func (b bot) getStatements(command string) ([]StatementItem, error) {

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

	req.Header.Add("x-token", b.monoToken)

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

	log.Printf("[monoapi] statements, body %s", string(body))

	if err := json.Unmarshal(body, &statementItems); err != nil {
		log.Printf("[monoapi] statements, unmarshal error %s", err)
		return statementItems, err
	}

	return statementItems, nil
}

func (b bot) getClientInfo() (ClientInfo, error) {
	var clientInfo ClientInfo

	url := "https://api.monobank.ua/personal/client-info"
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		log.Printf("[monoapi] client info, create request error %s", err)
		return clientInfo, err
	}

	req.Header.Add("x-token", b.monoToken)

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

func (b bot) isAdmin(userID int) bool {
	return b.checkIds(b.telegramAdmins, int64(userID))
}

func (b bot) isChat(chatID int64) bool {
	return b.checkIds(b.telegramChats, chatID)
}

func (b bot) checkIds(stringIds string, id int64) bool {
	ids := strings.Split(strings.Trim(stringIds, " "), ",")
	for _, _id := range ids {
		if reflect.DeepEqual([]byte(_id), []byte(strconv.FormatInt(id, 10))) {
			return true
		}
	}

	return false
}

func (b bot) normalizePrice(price int) string {
	return fmt.Sprintf("%.2f₴", float64(price/100))
}
