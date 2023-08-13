package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io/ioutil"
	"net/http"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/rs/zerolog/log"

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

// Bot is the interface representing bot object.
type Bot interface {
	InitMonoClients(monoTokens string) error
	TelegramStart(token string)
	WebhookStart()
	ProcessingStart()
	ScheduleReport(ctx context.Context) (int, error)
}

// bot is implementation the Bot interface
type bot struct {
	telegramAdmins string
	telegramChats  string
	clients        []Client

	BotAPI *tgbotapi.BotAPI

	ch chan StatementItemData

	statementTmpl *template.Template
	balanceTmpl   *template.Template
	webhookTmpl   *template.Template

	mono *Mono
}

// New returns a bot object.
func New(telegramAdmins, telegramChats string) Bot {

	statementTmpl, err := GetTempate(statementTemplate)
	if err != nil {
		log.Fatal().Err(err).Msg("[template]")
	}

	balanceTmpl, err := GetTempate(balanceTemplate)
	if err != nil {
		log.Fatal().Err(err).Msg("[template]")
	}

	webhookTmpl, err := GetTempate(webhookTemplate)
	if err != nil {
		log.Fatal().Err(err).Msg("[template]")
	}

	b := bot{
		telegramAdmins: telegramAdmins,
		telegramChats:  telegramChats,

		ch: make(chan StatementItemData, 100),

		statementTmpl: statementTmpl,
		balanceTmpl:   balanceTmpl,
		webhookTmpl:   webhookTmpl,
		mono:          NewMono(),
	}

	return &b
}

// InitMonoClients gets needed client data for correct working of the bot
func (b *bot) InitMonoClients(monoTokens string) error {

	monoTokensArr := strings.Split(monoTokens, ",")

	// init clients
	clients := make([]Client, 0, len(monoTokensArr))
	for _, monoToken := range monoTokensArr {

		client := NewClient(monoToken, b.mono)
		if err := client.Init(); err != nil {
			return err
		}

		clients = append(clients, client)
	}

	b.clients = clients

	return nil
}

// TelegramStart starts getting updates from telegram.
func (b *bot) TelegramStart(token string) {
	botAPI, err := tgbotapi.NewBotAPI(token)
	if err != nil {
		log.Panic().Err(err).Msg("[telegram] create bot")
	}

	b.BotAPI = botAPI

	log.Info().Msgf("Authorized on account %s", b.BotAPI.Self.UserName)

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	updates, err := b.BotAPI.GetUpdatesChan(u)
	if err != nil {
		log.Panic().Err(err).Msg("[telegram] get updates chan")
	}

	for update := range updates {
		if update.CallbackQuery == nil && update.Message == nil {
			log.Warn().Msg("[telegram] received incorrect updates")
			continue
		}

		var fromID int
		var chatID int64
		if update.Message != nil {
			fromID = update.Message.From.ID
			chatID = update.Message.Chat.ID
		} else {
			fromID = update.CallbackQuery.Message.ReplyToMessage.From.ID
			chatID = update.CallbackQuery.Message.ReplyToMessage.Chat.ID
		}

		log.Debug().Msgf("[telegram] received a message from %d in chat %d", fromID, chatID)

		if !(b.isAdmin(fromID) || b.isChat(chatID)) {
			if update.CallbackQuery != nil {
				_, err = b.BotAPI.AnswerCallbackQuery(tgbotapi.CallbackConfig{
					CallbackQueryID: update.CallbackQuery.ID,
					Text:            "Access denied",
				})
				if err != nil {
					log.Error().Err(err).Msg("[telegram] access denied, callback answer error")
				}
			}

			continue
		}

		if update.Message != nil && strings.HasPrefix(update.Message.Text, "/balance") {
			if len(b.clients) > 1 {
				_, err = b.BotAPI.Send(b.sendClientButtons("bc", update))
				if err != nil {
					log.Error().Err(err).Msg("[telegram] report send msg error")
				}
			} else {
				err := b.sendBalanceByClient(b.clients[0], update.Message)
				if err != nil {
					log.Error().Err(err).Msg("[telegram] balance, send msg error")
				}
			}

		} else if update.Message != nil && strings.HasPrefix(update.Message.Text, "/report") {
			log.Debug().Msg("[telegram] report")

			if len(b.clients) > 1 {
				_, err = b.BotAPI.Send(b.sendClientButtons("rc", update))
				if err != nil {
					log.Error().Err(err).Msg("[telegram] report send msg error")
				}
			} else {
				tmConfig, err := sendAccountButtonsMessage("ra", b.clients[0], *update.Message)
				if err != nil {
					log.Error().Err(err).Msg("[telegram] report send msg error")
				}

				_, err = b.BotAPI.Send(tmConfig)
				if err != nil {
					log.Error().Err(err).Msg("[telegram] report send msg error")
				}
			}
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
				b.BotAPI.Send(msg)
				continue
			}

			clientInfo, err := client.GetInfo()
			if err != nil {
				msg := tgbotapi.NewMessage(update.Message.Chat.ID, err.Error())
				b.BotAPI.Send(msg)
				continue
			}

			var tpl bytes.Buffer
			err = b.webhookTmpl.Execute(&tpl, clientInfo)
			if err != nil {
				log.Error().Err(err).Msg("[telegram] get webhook, template execute error")
				continue
			}
			message := tpl.String()

			msg := tgbotapi.NewMessage(update.Message.Chat.ID, message)
			msg.ReplyToMessageID = update.Message.MessageID

			_, err = b.BotAPI.Send(msg)
			if err != nil {
				log.Error().Err(err).Msg("[telegram] get webhook, send msg error")
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
				b.BotAPI.Send(msg)
				continue
			}

			client, err := b.getClient(idx)
			if err != nil {
				msg := tgbotapi.NewMessage(update.Message.Chat.ID, err.Error())
				b.BotAPI.Send(msg)
				continue
			}

			response, err := client.SetWebHook(r2[1])
			if err != nil {
				msg := tgbotapi.NewMessage(update.Message.Chat.ID, err.Error())
				b.BotAPI.Send(msg)
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
				log.Error().Err(err).Msg("[telegram] set webhook, send msg error")
			}
		} else if update.Message != nil {
			log.Warn().Msg("[telegram] the messuge unsupport")
		} else if update.CallbackQuery != nil {

			callbackQueryData := callbackQueryDataParser(update.CallbackQuery.Data)

			client, err := b.getClientByID(callbackQueryData.ClientID)
			if err != nil {
				msg := tgbotapi.NewMessage(update.CallbackQuery.Message.Chat.ID, err.Error())
				b.BotAPI.Send(msg)
				continue
			}

			if update.CallbackQuery.Data != "" && update.CallbackQuery.Data[:2] == "bc" {
				// balance
				message, err := b.buildBalanceByClient(client)
				if err != nil {
					log.Error().Err(err).Msg("[telegram] balance, send msg error")
					continue
				}

				messageConfig := tgbotapi.NewEditMessageText(
					update.CallbackQuery.Message.Chat.ID,
					update.CallbackQuery.Message.MessageID,
					message,
				)

				_, err = b.BotAPI.Send(messageConfig)
				if err != nil {
					log.Error().Err(err).Msg("[telegram] balance, send msg error")
				}
			} else if update.CallbackQuery.Data != "" && update.CallbackQuery.Data[:2] == "rc" {
				// report account

				mConfig, err := sendAccountButtonsEditMessage("ra", client, *update.CallbackQuery.Message)

				if err != nil {
					log.Error().Err(err).Msg("[telegram] report send msg error")
				}

				_, err = b.BotAPI.Send(mConfig)
				if err != nil {
					log.Error().Err(err).Msg("[telegram] report send msg error")
				}
			} else if update.CallbackQuery.Data != "" && update.CallbackQuery.Data[:2] == "ra" {
				account, err := client.GetAccountByID(callbackQueryData.Account)
				if err != nil {
					msg := tgbotapi.NewMessage(update.CallbackQuery.Message.Chat.ID, err.Error())
					b.BotAPI.Send(msg)
					continue
				}

				message := client.GetReport(account.ID).GetKeyboarButtonConfig(update, client.GetID())
				message.Text = fmt.Sprintf(
					"%s, %s%s\n%s",
					client.GetName(),
					NormalizePrice(account.Balance),
					GetCurrencySymbol(account.CurrencyCode),
					message.Text,
				)

				_, err = b.BotAPI.Send(message)
				if err != nil {
					log.Error().Err(err).Msg("[telegram] report send msg error")
				}
			} else if update.CallbackQuery.Data != "" && (update.CallbackQuery.Data[:2] == "rp" || update.CallbackQuery.Data[:2] == "rr") {
				// report
				log.Debug().Msg("[telegram] report grid page")

				account, err := client.GetAccountByID(callbackQueryData.Account)
				if err != nil {
					msg := tgbotapi.NewMessage(update.CallbackQuery.Message.Chat.ID, err.Error())
					b.BotAPI.Send(msg)

					log.Error().Err(err).Msg("[telegram] get account by ID")
					continue
				}

				if !client.GetReport(account.ID).IsExistGridData(update) {
					items, err := client.GetStatement(strings.ReplaceAll(callbackQueryData.Period, "_", " "), account.ID)
					if err != nil {
						msg := tgbotapi.NewMessage(update.CallbackQuery.Message.Chat.ID, err.Error())
						b.BotAPI.Send(msg)

						log.Error().Err(err).Msg("[telegram] report grid page get statements")
						continue
					}

					// reinit statements data if does not exist
					client.GetReport(account.ID).SetGridData(update, items)
				}

				var editMessage tgbotapi.Chattable

				if update.CallbackQuery.Data[:2] == "rp" {
					_editMessage := client.GetReport(account.ID).GetReportGrid(update, client.GetID())
					_editMessage.Text = fmt.Sprintf(
						"%s, %s%s, %s\n%s",
						client.GetName(),
						NormalizePrice(account.Balance),
						GetCurrencySymbol(account.CurrencyCode),
						strings.ReplaceAll(callbackQueryData.Period, "_", " "),
						_editMessage.Text,
					)
					editMessage = _editMessage

				} else {
					_editMessage, err := client.GetReport(account.ID).GetUpdatedReportGrid(update)
					if err != nil {
						_, err = b.BotAPI.AnswerCallbackQuery(tgbotapi.CallbackConfig{
							CallbackQueryID: update.CallbackQuery.ID,
							Text:            "Error :(",
						})
						if err != nil {
							log.Error().Err(err).Msg("[telegram] report grid send callback answer on update error")
						}
					}
					_editMessage.Text = fmt.Sprintf(
						"%s, %s%s, %s\n%s",
						client.GetName(),
						NormalizePrice(account.Balance),
						GetCurrencySymbol(account.CurrencyCode),
						strings.ReplaceAll(callbackQueryData.Period, "_", " "),
						_editMessage.Text,
					)
					editMessage = _editMessage
				}

				_, err = b.BotAPI.Send(editMessage)
				if err != nil {
					log.Error().Err(err).Msg("[telegram] report grid send error")
				}
			} else {
				log.Warn().Msg("[telegram] the messege unsupport")
			}

			_, err = b.BotAPI.AnswerCallbackQuery(tgbotapi.CallbackConfig{
				CallbackQueryID: update.CallbackQuery.ID,
			})
			if err != nil {
				log.Error().Err(err).Msg("[telegram] report grid send callback answer error")
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
			fmt.Fprintf(w, "Not Ok!")

			log.Error().Err(err).Msg("[webhook] read")
			return
		}

		log.Debug().Msgf("[webhook] body %s", string(body))

		var statementItemData StatementItemData
		if err := json.Unmarshal(body, &statementItemData); err != nil {
			fmt.Fprintf(w, "Not Ok!")

			log.Error().Err(err).Msg("[webhook] unmarshal")
			return
		}

		b.ch <- statementItemData

		fmt.Fprintf(w, "Ok!")
	})

	server := &http.Server{
		Addr:              ":8080",
		ReadHeaderTimeout: 5 * time.Minute,
	}

	err := server.ListenAndServe()
	if err != nil {
		log.Panic().Err(err).Msg("[webhook] serve")
	}
}

// ProcessingStart starts processing data that received from chennal.
func (b *bot) ProcessingStart() {

	for {
		statementItemData := <-b.ch

		client, err := b.getClientByAccountID(statementItemData.Data.Account)
		if err != nil {
			log.Error().Err(err).Msg("[processing] get client by account")
			continue
		}

		account, err := client.GetAccountByID(statementItemData.Data.Account)
		if err != nil {
			log.Error().Err(err).Msg("[processing] get account by id")
			continue
		}

		client.ResetReport(statementItemData.Data.Account)

		var tpl bytes.Buffer
		err = b.statementTmpl.Execute(&tpl, struct {
			Name          string
			StatementItem StatementItem
			Account       Account
		}{
			Name:          client.GetName(),
			StatementItem: statementItemData.Data.StatementItem,
			Account:       *account,
		})
		if err != nil {
			log.Error().Err(err).Msg("[processing] template execute error")
			continue
		}
		message := tpl.String()

		// to chat
		err = b.sendTo(b.telegramChats, message)
		if err != nil {
			log.Error().Err(err).Msg("[processing] send to chat")
			continue
		}

		// to admin
		err = b.sendTo(b.telegramAdmins, message)
		if err != nil {
			log.Error().Err(err).Msg("[processing] send to admin")
			continue
		}
	}
}

func (b *bot) sendClientButtons(prefix string, update tgbotapi.Update) tgbotapi.MessageConfig {
	buttons := []tgbotapi.InlineKeyboardButton{}

	for _, client := range b.clients {
		callbackData := callbackQueryDataBuilder(prefix, pageData{
			Page:     0,
			Period:   "",
			ChatID:   update.Message.Chat.ID,
			FromID:   update.Message.From.ID,
			ClientID: uint32(client.GetID()),
		})

		buttons = append(buttons, tgbotapi.InlineKeyboardButton{
			Text:         client.GetName(),
			CallbackData: &callbackData,
		})
	}

	inlineKeyboardMarkup := tgbotapi.NewInlineKeyboardMarkup(buttons)

	messageConfig := tgbotapi.MessageConfig{}
	messageConfig.Text = "Виберіть клієнта:"
	messageConfig.ChatID = update.Message.Chat.ID
	messageConfig.ReplyToMessageID = update.Message.MessageID
	messageConfig.ReplyMarkup = inlineKeyboardMarkup

	return messageConfig
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

func (b bot) getClientByID(id uint32) (Client, error) {
	for _, client := range b.clients {
		if client.GetID() == id {
			return client, nil
		}
	}

	return nil, errors.New("client does not found")
}

func (b bot) getClientByAccountID(id string) (Client, error) {
	for _, client := range b.clients {
		info, _ := client.GetInfo()
		for _, account := range info.Accounts {
			if account.ID == id {
				return client, nil
			}
		}
	}

	return nil, errors.New("client does not found")
}

func (b *bot) buildBalanceByClient(client Client) (string, error) {
	clientInfo, err := client.Clear().GetInfo()
	if err != nil {
		return "", err
	}

	var tpl bytes.Buffer
	err = b.balanceTmpl.Execute(&tpl, clientInfo)
	if err != nil {
		return "", err
	}

	return tpl.String(), err
}

func (b *bot) sendBalanceByClient(client Client, tgMessage *tgbotapi.Message) error {
	message, err := b.buildBalanceByClient(client)
	if err != nil {
		msg := tgbotapi.NewMessage(tgMessage.Chat.ID, err.Error())
		_, err = b.BotAPI.Send(msg)
		if err != nil {
			return err
		}
	}

	msg := tgbotapi.NewMessage(tgMessage.Chat.ID, message)
	msg.ReplyToMessageID = tgMessage.MessageID

	_, err = b.BotAPI.Send(msg)
	return err
}

func (b *bot) sendTo(chatIds, message string) error {
	ids := strings.Split(strings.Trim(chatIds, " "), ",")
	for _, id := range ids {
		chatID, err := strconv.ParseInt(id, 10, 64)
		if err != nil {
			return err
		}

		_, err = b.BotAPI.Send(tgbotapi.NewMessage(chatID, message))
		if err != nil {
			return err
		}
	}

	return nil
}

func sendAccountButtonsEditMessage(prefix string, client Client, message tgbotapi.Message) (*tgbotapi.EditMessageTextConfig, error) {
	messageConfig, inlineKeyboardMarkup, _ := buildAccountButtons[tgbotapi.EditMessageTextConfig](prefix, client, message)
	messageConfig.Text = fmt.Sprintf("%s\nВиберіть рахунок:", client.GetName())
	messageConfig.ChatID = message.Chat.ID
	messageConfig.MessageID = message.MessageID
	messageConfig.ReplyMarkup = inlineKeyboardMarkup

	return messageConfig, nil
}

func sendAccountButtonsMessage(prefix string, client Client, message tgbotapi.Message) (*tgbotapi.MessageConfig, error) {

	messageConfig, inlineKeyboardMarkup, _ := buildAccountButtons[tgbotapi.MessageConfig](prefix, client, message)
	messageConfig.Text = fmt.Sprintf("%s\nВиберіть рахунок:", client.GetName())
	messageConfig.ChatID = message.Chat.ID
	messageConfig.ReplyToMessageID = message.MessageID
	messageConfig.ReplyMarkup = inlineKeyboardMarkup

	return messageConfig, nil
}

func buildAccountButtons[V tgbotapi.EditMessageTextConfig | tgbotapi.MessageConfig](prefix string, client Client, message tgbotapi.Message) (*V, *tgbotapi.InlineKeyboardMarkup, error) {
	buttons := []tgbotapi.InlineKeyboardButton{}

	info, err := client.GetInfo()
	if err != nil {
		return nil, nil, err
	}

	for _, account := range info.Accounts {
		callbackData := callbackQueryDataBuilder(prefix, pageData{
			Page:     0,
			Period:   "",
			ChatID:   message.Chat.ID,
			FromID:   message.From.ID,
			ClientID: uint32(client.GetID()),
			Account:  account.ID,
		})

		buttons = append(buttons, tgbotapi.InlineKeyboardButton{
			Text:         account.GetName(),
			CallbackData: &callbackData,
		})
	}

	inlineKeyboardMarkup := tgbotapi.NewInlineKeyboardMarkup(buttons)
	return new(V), &inlineKeyboardMarkup, nil
}
