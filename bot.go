package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io/ioutil"
	"log"
	"net/http"
	"reflect"
	"strconv"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api"
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
	Name       string    `json:"name"`
	WebHookURL string    `json:"webHookUrl,omitempty"`
	Accounts   []Account `json:"accounts"`
}

// Bot is the interface representing bot object.
type Bot interface {
	TelegramStart()
	WebhookStart()
	ProcessingStart()
}

// bot is implementation the Bot interface
type bot struct {
	telegramToken  string
	telegramAdmins string
	telegramChats  string
	clients        []Client

	BotAPI *tgbotapi.BotAPI

	ch chan StatementItemData

	statementTmpl *template.Template
	balanceTmpl   *template.Template
	webhookTmpl   *template.Template
}

// New returns a bot object.
func New(telegramToken, telegramAdmins, telegramChats, monoTokens string) Bot {

	statementTmpl, err := GetTempate(statementTemplate)
	if err != nil {
		log.Fatalf("[template] %s", err)
	}

	balanceTmpl, err := GetTempate(balanceTemplate)
	if err != nil {
		log.Fatalf("[template] %s", err)
	}

	webhookTmpl, err := GetTempate(webhookTemplate)
	if err != nil {
		log.Fatalf("[template] %s", err)
	}

	monoTokensArr := strings.Split(monoTokens, ",")

	// init clients
	clients := make([]Client, 0, len(monoTokensArr))
	for _, monoToken := range monoTokensArr {
		clients = append(clients, NewClient(monoToken))
	}

	b := bot{
		telegramToken:  telegramToken,
		telegramAdmins: telegramAdmins,
		telegramChats:  telegramChats,
		clients:        clients,

		ch: make(chan StatementItemData, 100),

		statementTmpl: statementTmpl,
		balanceTmpl:   balanceTmpl,
		webhookTmpl:   webhookTmpl,
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

		var fromID int
		var chatID int64
		if update.Message != nil {
			fromID = update.Message.From.ID
			chatID = update.Message.Chat.ID
		} else {
			fromID = update.CallbackQuery.Message.ReplyToMessage.From.ID
			chatID = update.CallbackQuery.Message.ReplyToMessage.Chat.ID
		}

		if !(b.isAdmin(fromID) || b.isChat(chatID)) {
			if update.CallbackQuery != nil {
				_, err = b.BotAPI.AnswerCallbackQuery(tgbotapi.CallbackConfig{
					CallbackQueryID: update.CallbackQuery.ID,
					Text:            "Access denied",
				})
				if err != nil {
					log.Printf("[telegram] access denied, callback answer error %s", err)
				}
			}

			continue
		}

		if update.Message != nil {
			log.Printf("[telegram] received a message from %d in chat %d",
				update.Message.From.ID, update.Message.Chat.ID)
		}

		if update.Message != nil && strings.HasPrefix(update.Message.Text, "/balance") {

			r1 := strings.Split(strings.TrimPrefix(update.Message.Text, "/balance"), "_")
			idx := 0
			if len(r1) == 1 {
				idx = 0
			} else {
				idx, _ = strconv.Atoi(r1[1])
			}

			client, err := b.getClient(idx)
			if err != nil {
				msg := tgbotapi.NewMessage(update.Message.Chat.ID, err.Error())
				_, err = b.BotAPI.Send(msg)
				continue
			}

			clientInfo, err := client.GetInfo()
			if err != nil {
				msg := tgbotapi.NewMessage(update.Message.Chat.ID, err.Error())
				_, err = b.BotAPI.Send(msg)
				continue
			}

			var account Account
			for _, _account := range clientInfo.Accounts {
				if _account.CurrencyCode == 980 {
					account = _account
				}
			}

			var tpl bytes.Buffer
			err = b.balanceTmpl.Execute(&tpl, account)
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
		} else if update.Message != nil && strings.HasPrefix(update.Message.Text, "/report") {
			log.Printf("[telegram] report show keyboard")

			r1 := strings.Split(strings.TrimPrefix(update.Message.Text, "/report"), "_")
			idx := 0
			if len(r1) == 1 {
				idx = 0
			} else {
				idx, _ = strconv.Atoi(r1[1])
			}

			client, err := b.getClient(idx)
			if err != nil {
				msg := tgbotapi.NewMessage(update.CallbackQuery.Message.Chat.ID, err.Error())
				_, err = b.BotAPI.Send(msg)
				continue
			}

			_, err = b.BotAPI.Send(client.GetReport().GetKeyboardMessageConfig(update))
			if err != nil {
				log.Printf("[telegram] report send msg error %s", err)
			}

			if err != nil {
				log.Printf("[telegram] report send msg error %s", err)
			}

			// set state of the client
			client.SetState(ClientStateReport)
		} else if update.Message != nil && strings.HasPrefix(update.Message.Text, "/get_webhook") {

			r1 := strings.Split(strings.TrimPrefix(update.Message.Text, "/get_webhook"), "_")
			idx := 0
			if len(r1) == 1 {
				idx = 0
			} else {
				idx, _ = strconv.Atoi(r1[1])
			}

			client, err := b.getClient(idx)
			if err != nil {
				msg := tgbotapi.NewMessage(update.Message.Chat.ID, err.Error())
				_, err = b.BotAPI.Send(msg)
				continue
			}

			clientInfo, err := client.GetInfo()
			if err != nil {
				msg := tgbotapi.NewMessage(update.Message.Chat.ID, err.Error())
				_, err = b.BotAPI.Send(msg)
				continue
			}

			var tpl bytes.Buffer
			err = b.webhookTmpl.Execute(&tpl, clientInfo)
			if err != nil {
				log.Printf("[telegram] get webhook, template execute error %s", err)
				continue
			}
			message := tpl.String()

			msg := tgbotapi.NewMessage(update.Message.Chat.ID, message)
			msg.ReplyToMessageID = update.Message.MessageID

			_, err = b.BotAPI.Send(msg)
			if err != nil {
				log.Printf("[telegram] balance, send msg error %s", err)
			}
		} else if update.Message != nil && strings.HasPrefix(update.Message.Text, "/set_webhook") {

			r0 := strings.TrimPrefix(update.Message.Text, "/set_webhook")
			r2 := strings.Split(r0, " ")

			if len(r2) != 2 {
				continue
			}

			r1 := strings.Split(r2[0], "_")

			idx := 0
			if len(r1) == 1 {
				idx = 0
			} else {
				idx, _ = strconv.Atoi(r1[1])
			}

			if !IsURL(r2[1]) {
				msg := tgbotapi.NewMessage(update.Message.Chat.ID, "Incorrect url")
				_, err = b.BotAPI.Send(msg)
				continue
			}

			client, err := b.getClient(idx)
			if err != nil {
				msg := tgbotapi.NewMessage(update.Message.Chat.ID, err.Error())
				_, err = b.BotAPI.Send(msg)
				continue
			}

			response, err := client.SetWebHook(r2[1])
			if err != nil {
				msg := tgbotapi.NewMessage(update.Message.Chat.ID, err.Error())
				_, err = b.BotAPI.Send(msg)
				continue
			}

			message := response.Status
			if message == "" {
				message = fmt.Sprintf("error: %s", response.ErrorDescription)
			}

			msg := tgbotapi.NewMessage(update.Message.Chat.ID, message)
			msg.ReplyToMessageID = update.Message.MessageID

			_, err = b.BotAPI.Send(msg)
			if err != nil {
				log.Printf("[telegram] balance, send msg error %s", err)
			}
		} else if update.Message != nil {

			client, err := b.getClientByState(ClientStateReport)
			if err != nil {
				msg := tgbotapi.NewMessage(update.Message.Chat.ID, err.Error())
				_, err = b.BotAPI.Send(msg)
				continue
			}

			log.Printf("[telegram] report grid")

			items, err := client.GetStatement(client.GetReport().GetPeriodFromUpdate(update))
			if err != nil {
				log.Printf("[telegram] report get statements error %s", err)

				msg := tgbotapi.NewMessage(update.Message.Chat.ID, err.Error())
				_, err = b.BotAPI.Send(msg)
				continue
			}

			// set statement data
			client.GetReport().SetGridData(update, items)

			_, err = b.BotAPI.Send(client.GetReport().GetReportGrid(update, client.GetID()))
			if err != nil {
				log.Printf("[telegram] report grid send error %s", err)
			}

			messageConfig := tgbotapi.MessageConfig{}
			messageConfig.Text = "ᅠ "
			messageConfig.ChatID = update.Message.Chat.ID
			messageConfig.ReplyMarkup = tgbotapi.NewRemoveKeyboard(false)
			_, err = b.BotAPI.Send(messageConfig)
			if err != nil {
				log.Printf("[telegram] remove keyboard error %s", err)
			}

		} else if update.CallbackQuery != nil {

			client, err := b.getClientByID(callbackQueryDataParser(update.CallbackQuery.Data).ClientID)
			if err != nil {
				msg := tgbotapi.NewMessage(update.CallbackQuery.Message.Chat.ID, err.Error())
				_, err = b.BotAPI.Send(msg)
				continue
			}

			log.Printf("[telegram] report grid page")

			if !client.GetReport().IsExistGridData(update) {
				items, err := client.GetStatement(client.GetReport().GetPeriodFromUpdate(update))
				if err != nil {
					log.Printf("[telegram] report grid page get statements error %s", err)

					msg := tgbotapi.NewMessage(update.CallbackQuery.Message.Chat.ID, err.Error())
					_, err = b.BotAPI.Send(msg)
					continue
				}

				// reinit statements data if does not exist
				client.GetReport().SetGridData(update, items)
			}

			editMessage, err := client.GetReport().GetUpdatedReportGrid(update)
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

			// set state of the client
			client.SetState(ClientStateNone)
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

		b.ch <- statementItemData

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
		case statementItemData := <-b.ch:
			var tpl bytes.Buffer
			err := b.statementTmpl.Execute(&tpl, statementItemData.Data.StatementItem)
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

func (b *bot) getClient(index int) (Client, error) {
	if len(b.clients) > index {
		return b.clients[index], nil
	}

	return nil, errors.New("Client does not found")
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

func (b bot) getClientByState(state ClientState) (Client, error) {
	for _, client := range b.clients {
		if client.IsState(state) {
			return client, nil
		}
	}

	return nil, errors.New("please repeat a command for client")
}

func (b bot) getClientByID(id uint32) (Client, error) {
	for _, client := range b.clients {
		if client.GetID() == id {
			return client, nil
		}
	}

	return nil, errors.New("client does not found")
}

func (b bot) normalizePrice(price int) string {
	return fmt.Sprintf("%.2f₴", float64(price/100))
}
