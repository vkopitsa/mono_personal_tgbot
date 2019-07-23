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

		// var fromID int
		// var chatID int64
		// if update.Message != nil {
		// 	fromID = update.Message.From.ID
		// 	chatID = update.Message.Chat.ID
		// }

		// if !(b.isAdmin(update.Message.From.ID) || b.isChat(update.Message.Chat.ID)) {
		// 	continue
		// }

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

	// // TODO: remove
	// if err := json.Unmarshal([]byte(`[{"id":"CM9hyuylV9TaiBo","time":1563895099,"description":"АТБ","mcc":5411,"amount":-20980,"operationAmount":-20980,"currencyCode":980,"commissionRate":0,"cashbackAmount":419,"balance":219174,"hold":true},{"id":"tg84J_KO_tXzl2c","time":1563889412,"description":"Мой приват гр","mcc":4829,"amount":-30000,"operationAmount":-30000,"currencyCode":980,"commissionRate":0,"cashbackAmount":0,"balance":240154,"hold":true},{"id":"C82CIdXtnwfrsiI","time":1563889335,"description":"От: ФОП КОПИЦЯ ВОЛОДИМИР ВІКТОРОВИЧ","mcc":4829,"amount":270000,"operationAmount":270000,"currencyCode":980,"commissionRate":0,"cashbackAmount":0,"balance":270154,"hold":true},{"id":"8bT7TXUj1K-8yyc","time":1563733691,"description":"Банкомат DN00 Illinska  str., b","mcc":6011,"amount":-20100,"operationAmount":-20100,"currencyCode":980,"commissionRate":100,"cashbackAmount":0,"balance":154,"hold":true},{"id":"1uWDxailk_BbakI","time":1563733639,"description":"От: Світлана Копиця","mcc":4829,"amount":200,"operationAmount":200,"currencyCode":980,"commissionRate":0,"cashbackAmount":0,"balance":20254,"hold":true},{"id":"RFTBYbhGR_ll_QM","time":1563733616,"description":"От: Світлана Копиця","mcc":4829,"amount":900,"operationAmount":900,"currencyCode":980,"commissionRate":0,"cashbackAmount":0,"balance":20054,"hold":true},{"id":"rDaBsvQAlWNm--Y","time":1563733534,"description":"Банкомат DN00 Illinska  str., b","mcc":6011,"amount":-20100,"operationAmount":-20100,"currencyCode":980,"commissionRate":100,"cashbackAmount":0,"balance":19154,"hold":true},{"id":"-OCUS9Dj8MLh2KQ","time":1563730703,"description":"Світлана Копиця","mcc":4829,"amount":-100,"operationAmount":-100,"currencyCode":980,"commissionRate":0,"cashbackAmount":0,"balance":39254,"hold":true},{"id":"RK_lBt3GO8qN4xc","time":1563694816,"description":"От: Світлана Копиця","comment":"Возврат","mcc":4829,"amount":100,"operationAmount":100,"currencyCode":980,"commissionRate":0,"cashbackAmount":0,"balance":39354,"hold":true},{"id":"61GekshIIBmguG4","time":1563694411,"description":"Світлана Копиця","mcc":4829,"amount":-100,"operationAmount":-100,"currencyCode":980,"commissionRate":0,"cashbackAmount":0,"balance":39254,"hold":true},{"id":"K-BX2nrSs-fdjjU","time":1563692281,"description":"АТБ","mcc":5411,"amount":-19730,"operationAmount":-19730,"currencyCode":980,"commissionRate":0,"cashbackAmount":394,"balance":39354,"hold":false},{"id":"ioorhXdsSJ_Ul3M","time":1563623368,"description":"McDonalds","mcc":5814,"amount":-26600,"operationAmount":-26600,"currencyCode":980,"commissionRate":0,"cashbackAmount":798,"balance":59084,"hold":true},{"id":"4Vg01vtZCeicFwk","time":1563563811,"description":"От: Світлана Копиця","comment":"На тобі, жлоб","mcc":4829,"amount":100,"operationAmount":100,"currencyCode":980,"commissionRate":0,"cashbackAmount":0,"balance":85684,"hold":true},{"id":"HT_5XEK6-oMa9PU","time":1563563742,"description":"Світлана Копиця","comment":"I'm waiting for my money 💵","mcc":4829,"amount":-100,"operationAmount":-100,"currencyCode":980,"commissionRate":0,"cashbackAmount":0,"balance":85584,"hold":true},{"id":"xG3TspFBKLsSfJc","time":1563563220,"description":"Світлана Копиця","comment":"Please back the amount with anything comment","mcc":4829,"amount":-100,"operationAmount":-100,"currencyCode":980,"commissionRate":0,"cashbackAmount":0,"balance":85684,"hold":true},{"id":"4z7j-aU-EOLqaQY","time":1563561107,"description":"От: Світлана Копиця","comment":"Піздюку коханому :)","mcc":4829,"amount":100,"operationAmount":100,"currencyCode":980,"commissionRate":0,"cashbackAmount":0,"balance":85784,"hold":true},{"id":"cvj2VNJtLgdtQfU","time":1563558640,"description":"VCODE*452902","mcc":8999,"amount":-100,"operationAmount":-100,"currencyCode":980,"commissionRate":0,"cashbackAmount":0,"balance":85684,"hold":true},{"id":"nT-MM1EXibakbkw","time":1563554488,"description":"Сільпо","mcc":5411,"amount":-21554,"operationAmount":-21554,"currencyCode":980,"commissionRate":0,"cashbackAmount":431,"balance":85784,"hold":false},{"id":"b7MR5PBCpivtWNQ","time":1563554181,"description":"FOP REZNIK P-59 K-1","mcc":5499,"amount":-6070,"operationAmount":-6070,"currencyCode":980,"commissionRate":0,"cashbackAmount":121,"balance":107338,"hold":false},{"id":"iku7NezLVthiA1s","time":1563553353,"description":"HOUSE SUMY","mcc":5651,"amount":-9900,"operationAmount":-9900,"currencyCode":980,"commissionRate":0,"cashbackAmount":0,"balance":113408,"hold":false},{"id":"pOIrC1VAA2068r4","time":1563552016,"description":"S.UA.03.62","mcc":5655,"amount":-14500,"operationAmount":-14500,"currencyCode":980,"commissionRate":0,"cashbackAmount":0,"balance":123308,"hold":false},{"id":"DZJeyPCNFD4ZNQQ","time":1563541480,"description":"От: Світлана Копиця","mcc":4829,"amount":100,"operationAmount":100,"currencyCode":980,"commissionRate":0,"cashbackAmount":0,"balance":137808,"hold":true},{"id":"uxHwgouS3Vb43DU","time":1563541415,"description":"Светулька","mcc":4829,"amount":-100,"operationAmount":-100,"currencyCode":980,"commissionRate":0,"cashbackAmount":0,"balance":137708,"hold":true},{"id":"hGMkk1jdpmyW9eA","time":1563540895,"description":"Светулька","mcc":4829,"amount":-100,"operationAmount":-100,"currencyCode":980,"commissionRate":0,"cashbackAmount":0,"balance":137808,"hold":true},{"id":"ekigN0tNJeQ5QtE","time":1563540573,"description":"Светулька","mcc":4829,"amount":-100,"operationAmount":-100,"currencyCode":980,"commissionRate":0,"cashbackAmount":0,"balance":137908,"hold":true},{"id":"e1SKgXyxlsvDLYY","time":1563518600,"description":"FOP REZNIK P-59 K-2","mcc":5499,"amount":-2550,"operationAmount":-2550,"currencyCode":980,"commissionRate":0,"cashbackAmount":51,"balance":138008,"hold":false},{"id":"tHCyaxSVvSzbyCw","time":1563469747,"description":"iHerb","mcc":5499,"amount":-169706,"operationAmount":-169706,"currencyCode":980,"commissionRate":0,"cashbackAmount":3394,"balance":140558,"hold":false},{"id":"KeGctBeJeiTxujg","time":1563467850,"description":"Банкомат DN00 Stepana Bandery","mcc":6011,"amount":-100500,"operationAmount":-100500,"currencyCode":980,"commissionRate":500,"cashbackAmount":0,"balance":310264,"hold":false},{"id":"D1IWZ2CdMirVeTg","time":1563462870,"description":"516875****7373","mcc":4829,"amount":-1000,"operationAmount":-1000,"currencyCode":980,"commissionRate":0,"cashbackAmount":0,"balance":410764,"hold":true},{"id":"anu3P3fb82OhIvI","time":1563458881,"description":"Мама","mcc":4829,"amount":-100000,"operationAmount":-100000,"currencyCode":980,"commissionRate":0,"cashbackAmount":0,"balance":411764,"hold":true},{"id":"cjT5nvGt2fNNcmU","time":1563458069,"description":"От: ФОП КОПИЦЯ ВОЛОДИМИР ВІКТОРОВИЧ","mcc":4829,"amount":500000,"operationAmount":500000,"currencyCode":980,"commissionRate":0,"cashbackAmount":0,"balance":511764,"hold":true},{"id":"xlyUOJ2UEHfhDBY","time":1563455562,"description":"516930****3367","mcc":4829,"amount":-1000,"operationAmount":-1000,"currencyCode":980,"commissionRate":0,"cashbackAmount":0,"balance":11764,"hold":true},{"id":"1HSv8YG07Z8jldI","time":1563455060,"description":"От: ФОП КОПИЦЯ ВОЛОДИМИР ВІКТОРОВИЧ","mcc":4829,"amount":10000,"operationAmount":10000,"currencyCode":980,"commissionRate":0,"cashbackAmount":0,"balance":12764,"hold":true},{"id":"iOldn69c7vhUZ7w","time":1563450095,"description":"Максим Рева","mcc":4829,"amount":-4000,"operationAmount":-4000,"currencyCode":980,"commissionRate":0,"cashbackAmount":0,"balance":2764,"hold":true},{"id":"LttPvObRqxwuswU","time":1563267413,"description":"Vodafone\n+380668337598","mcc":4814,"amount":-10000,"operationAmount":-10000,"currencyCode":980,"commissionRate":0,"cashbackAmount":0,"balance":6764,"hold":true},{"id":"BujAa9viDjaI5QQ","time":1563267348,"description":"От: Мой укрсиб","mcc":4829,"amount":10000,"operationAmount":10000,"currencyCode":980,"commissionRate":0,"cashbackAmount":0,"balance":16764,"hold":true},{"id":"xyvlVJpLi57bFmc","time":1563263508,"description":"WWW.HETZNER.DE","mcc":7399,"amount":-14364,"operationAmount":-490,"currencyCode":978,"commissionRate":0,"cashbackAmount":0,"balance":6764,"hold":false},{"id":"bqEj_1g0o6hOlxU","time":1563198834,"description":"От: Сергій Харченко","mcc":4829,"amount":100,"operationAmount":100,"currencyCode":980,"commissionRate":0,"cashbackAmount":0,"balance":21128,"hold":true},{"id":"0zDFdOJLTwd7ZSQ","time":1563198781,"description":"Сергій Харченко","comment":"Test webhook","mcc":4829,"amount":-100,"operationAmount":-100,"currencyCode":980,"commissionRate":0,"cashbackAmount":0,"balance":21028,"hold":true},{"id":"M5dqXdqPjm6UrFQ","time":1563186155,"description":"От: Сергій Харченко","comment":"Test WH","mcc":4829,"amount":100,"operationAmount":100,"currencyCode":980,"commissionRate":0,"cashbackAmount":0,"balance":21128,"hold":true},{"id":"buCK3jm4nRFZKJo","time":1563186106,"description":"Сергій Харченко","comment":"Test webhook","mcc":4829,"amount":-100,"operationAmount":-100,"currencyCode":980,"commissionRate":0,"cashbackAmount":0,"balance":21028,"hold":true},{"id":"RIjWoR_G0DmdjQ0","time":1563162373,"description":"От: Мой укрсиб","mcc":4829,"amount":20000,"operationAmount":20000,"currencyCode":980,"commissionRate":0,"cashbackAmount":0,"balance":21128,"hold":true}]`), &statementItems); err != nil {
	// 	log.Printf("[monoapi] statements, unmarshal error %s", err)
	// 	return statementItems, err
	// }
	// //log.Printf("[monoapi] statements, array %s", statementItems)
	// return statementItems, nil
	// // TODO: remove

	url := fmt.Sprintf("https://api.monobank.ua/personal/statement/0/%d", from)
	if to > 0 {
		url = fmt.Sprintf("%s/%d", url, to)
	}
	//url := fmt.Sprintf("https://api.monobank.ua/personal/statement/0/1561939200")

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
