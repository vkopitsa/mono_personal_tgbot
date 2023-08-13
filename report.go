package main

import (
	"bytes"
	"fmt"
	"html/template"
	"strings"

	"github.com/rs/zerolog/log"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api"

	"time"
)

var reportCommant = map[string]string{
	// manual
	"Today":      "Today",
	"This week":  "This week",
	"Last week":  "Last week",
	"This month": "This month",
	"Last month": "Last month",
	// months
	"January":   "1",
	"February":  "2",
	"March":     "3",
	"April":     "4",
	"May":       "5",
	"June":      "6",
	"July":      "7",
	"August":    "8",
	"September": "9",
	"October":   "10",
	"November":  "11",
	"December":  "12",
}

// Report is the interface representing report object.
type Report interface {
	GetKeyboarButtonConfig(update tgbotapi.Update, clientID uint32) tgbotapi.EditMessageTextConfig
	IsReportGridCommand(update tgbotapi.Update) bool
	IsReportGridPageCommand(update tgbotapi.Update) bool
	GetReportGrid(update tgbotapi.Update, clientID uint32) tgbotapi.EditMessageTextConfig
	GetUpdatedReportGrid(update tgbotapi.Update) (tgbotapi.EditMessageTextConfig, error)
	IsExistGridData(update tgbotapi.Update) bool
	SetGridData(update tgbotapi.Update, items []StatementItem)
	GetPeriodFromUpdate(update tgbotapi.Update) string
	ResetLastData()
	IsAccount(accountId string) bool
}

type report struct {
	cache map[string][]StatementItem

	prefix   string
	perPage  int
	tmpl     *template.Template
	account  *Account
	clientId uint32
}

// ReportPage is a structure to render  report content the telegram
type ReportPage struct {
	StatementItems      []StatementItem // items per page
	SpentTotal          int             // total for the period
	AmountTotal         int             // total for the period
	CurrencyCode        int             // total for the period
	CashbackAmountTotal int             // total for the period
	Period              string
}

// NewReport returns a report object.
func NewReport(account *Account, clientId uint32) Report {

	tmpl, err := GetTempate(reportPageTemplate)
	if err != nil {
		log.Fatal().Err(err).Msg("[template]")
	}

	return &report{
		prefix:   "rr",
		perPage:  5,
		cache:    map[string][]StatementItem{},
		tmpl:     tmpl,
		account:  account,
		clientId: clientId,
	}
}

func (r *report) getCacheKay(update tgbotapi.Update) string {
	var key string

	var fromID int
	var chatID int64
	var clientID uint32

	if update.Message != nil {
		key = update.Message.Text

		fromID = update.Message.From.ID
		chatID = update.Message.Chat.ID
	} else {
		data := callbackQueryDataParser(update.CallbackQuery.Data)
		key = data.Period
		clientID = data.ClientID

		fromID = update.CallbackQuery.Message.ReplyToMessage.From.ID
		chatID = update.CallbackQuery.Message.ReplyToMessage.Chat.ID
	}

	return fmt.Sprintf(
		"%s-report-%d-%d-%d",
		key,
		chatID, fromID, clientID,
	)
}

func (r *report) SetGridData(update tgbotapi.Update, items []StatementItem) {
	r.cache[r.getCacheKay(update)] = items
}

func (r report) IsExistGridData(update tgbotapi.Update) bool {
	_, ok := r.cache[r.getCacheKay(update)]
	return ok
}

func (r *report) GetReportGrid(update tgbotapi.Update, clientID uint32) tgbotapi.EditMessageTextConfig {
	items := r.cache[r.getCacheKay(update)]
	data := callbackQueryDataParser(update.CallbackQuery.Data)

	var tpl bytes.Buffer
	err := r.tmpl.Execute(&tpl, r.buildReportPage(items, 1, r.perPage))
	if err != nil {
		log.Error().Err(err).Msg("[processing] template execute error")
		return tgbotapi.EditMessageTextConfig{}
	}
	message := tpl.String()

	tgMessage := update.Message
	if tgMessage == nil && update.CallbackQuery != nil {
		tgMessage = update.CallbackQuery.Message
	}

	inlineKeyboardMarkup := tgbotapi.NewInlineKeyboardMarkup(
		getPaginateButtons(len(items), 1, r.perPage, callbackQueryDataBuilder(r.prefix, data)))

	messageConfig := tgbotapi.EditMessageTextConfig{}
	messageConfig.Text = message
	messageConfig.ChatID = tgMessage.Chat.ID
	// messageConfig.ReplyToMessageID = tgMessage.MessageID
	messageConfig.MessageID = tgMessage.MessageID
	messageConfig.ReplyMarkup = &inlineKeyboardMarkup

	return messageConfig
}

func (r report) GetUpdatedReportGrid(update tgbotapi.Update) (tgbotapi.EditMessageTextConfig, error) {
	items := r.cache[r.getCacheKay(update)]
	data := callbackQueryDataParser(update.CallbackQuery.Data)

	var tpl bytes.Buffer
	err := r.tmpl.Execute(&tpl, r.buildReportPage(items, data.Page, r.perPage))
	if err != nil {
		log.Error().Err(err).Msg("[processing] template execute error")
		return tgbotapi.EditMessageTextConfig{}, err
	}
	message := tpl.String()

	inlineKeyboardMarkup := tgbotapi.NewInlineKeyboardMarkup(
		getPaginateButtons(
			len(items),
			data.Page,
			r.perPage,
			callbackQueryDataBuilder(r.prefix, data),
		),
	)

	messageConfig := tgbotapi.NewEditMessageText(
		update.CallbackQuery.Message.Chat.ID,
		update.CallbackQuery.Message.MessageID,
		message,
	)

	messageConfig.ReplyMarkup = &inlineKeyboardMarkup

	return messageConfig, nil
}

func (r report) buildReportPage(items []StatementItem, page, limit int) ReportPage {
	total := len(items)
	totalPages := int(total / limit)
	if total%limit != 0 {
		totalPages++
	}

	var amountTotal int
	var cashbackAmountTotal int
	var spentTotal int

	for _, item := range items {
		if item.Amount < 0 {
			spentTotal += -item.Amount
		}
		amountTotal += abs(item.Amount)
		cashbackAmountTotal += item.CashbackAmount
	}

	if total > 0 {
		if page == 1 && len(items) >= limit {
			items = items[:limit]
		} else if totalPages == page {
			items = items[(totalPages-1)*limit:]
		} else {
			items = items[(page-1)*limit : page*limit]
		}
	}

	return ReportPage{
		StatementItems:      items,
		AmountTotal:         amountTotal,
		CurrencyCode:        r.account.CurrencyCode,
		SpentTotal:          spentTotal,
		CashbackAmountTotal: cashbackAmountTotal,
	}
}

func (r report) IsReportGridPageCommand(update tgbotapi.Update) bool {
	data := update.CallbackQuery.Data
	return strings.HasPrefix(data, r.prefix)
}

func (r report) IsReportGridCommand(update tgbotapi.Update) bool {
	_, ok := reportCommant[update.Message.Text]
	return ok
}

func (r report) GetKeyboarButtonConfig(update tgbotapi.Update, clientID uint32) tgbotapi.EditMessageTextConfig {
	tgMessage := update.Message
	if tgMessage == nil && update.CallbackQuery != nil {
		tgMessage = update.CallbackQuery.Message
	}

	callbackQueryDataPerion := func(p string) *string {
		d := callbackQueryDataBuilder("rp", pageData{
			// Page:     1,
			Period:   strings.ReplaceAll(p, " ", "_"),
			ChatID:   tgMessage.Chat.ID,
			FromID:   tgMessage.From.ID,
			ClientID: r.clientId,
			Account:  r.account.ID,
		})

		// add page number
		d = d + "1"

		return &d
	}

	custom := []tgbotapi.InlineKeyboardButton{
		{
			Text:         "Today",
			CallbackData: callbackQueryDataPerion("Today"),
		},
		{
			Text:         "This week",
			CallbackData: callbackQueryDataPerion("This week"),
		},
		{
			Text:         "Last week",
			CallbackData: callbackQueryDataPerion("Last week"),
		},
		{
			Text:         "This month",
			CallbackData: callbackQueryDataPerion("This month"),
		},
		{
			Text:         "Last month",
			CallbackData: callbackQueryDataPerion("Last month"),
		},
	}
	months := []tgbotapi.InlineKeyboardButton{
		{
			Text:         "January",
			CallbackData: callbackQueryDataPerion("January"),
		},
		{
			Text:         "February",
			CallbackData: callbackQueryDataPerion("February"),
		},
		{
			Text:         "March",
			CallbackData: callbackQueryDataPerion("March"),
		},
		{
			Text:         "April",
			CallbackData: callbackQueryDataPerion("April"),
		},
		{
			Text:         "May",
			CallbackData: callbackQueryDataPerion("May"),
		},
		{
			Text:         "June",
			CallbackData: callbackQueryDataPerion("June"),
		},
	}
	months2 := []tgbotapi.InlineKeyboardButton{
		{
			Text:         "July",
			CallbackData: callbackQueryDataPerion("July"),
		},
		{
			Text:         "August",
			CallbackData: callbackQueryDataPerion("August"),
		},
		{
			Text:         "September",
			CallbackData: callbackQueryDataPerion("September"),
		},
		{
			Text:         "October",
			CallbackData: callbackQueryDataPerion("October"),
		},
		{
			Text:         "November",
			CallbackData: callbackQueryDataPerion("November"),
		},
		{
			Text:         "December",
			CallbackData: callbackQueryDataPerion("December"),
		},
	}

	month := time.Now().Month()
	if int(month) < len(months) {
		months = months[:month]
	}
	if int(month) > len(months2) {
		months2 = months2[:month-6]
	}

	inlineKeyboardMarkup := tgbotapi.NewInlineKeyboardMarkup(custom, months, months2)

	messageConfig := tgbotapi.EditMessageTextConfig{}
	messageConfig.Text = "Виберіть період"
	messageConfig.ChatID = tgMessage.Chat.ID
	messageConfig.MessageID = tgMessage.MessageID
	messageConfig.ReplyMarkup = &inlineKeyboardMarkup

	return messageConfig
}

func (r report) GetPeriodFromUpdate(update tgbotapi.Update) string {
	if update.Message != nil {
		return update.Message.Text
	} else if update.CallbackQuery != nil {
		return update.CallbackQuery.Message.ReplyToMessage.Text
	}

	return ""
}

func (r *report) ResetLastData() {

	keys := []string{"Today", "This week", "Last week", "This month", "Last month"}

	_, month, _ := time.Now().Date()
	keys = append(keys, month.String())

	for cacheKey := range r.cache {
		for _, key := range keys {
			if strings.Contains(cacheKey, key) {
				delete(r.cache, cacheKey)
			}
		}
	}
}

func (r *report) IsAccount(accountId string) bool {
	return r.account.ID == accountId
}
