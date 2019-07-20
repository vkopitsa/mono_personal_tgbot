package main

import (
	"bytes"
	"fmt"
	"html/template"
	"log"
	"strconv"
	"strings"

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
	GetKeyboardMessageConfig(update tgbotapi.Update) tgbotapi.MessageConfig
	IsReportGridCommand(update tgbotapi.Update) bool
	IsReportGridPageCommand(update tgbotapi.Update) bool
	GetReportGrid(update tgbotapi.Update) tgbotapi.MessageConfig
	GetUpdatedReportGrid(update tgbotapi.Update) (tgbotapi.EditMessageTextConfig, error)
	IsExistGridData(update tgbotapi.Update) bool
	SetGridData(update tgbotapi.Update, items []StatementItem)
	GetPeriodFromUpdate(update tgbotapi.Update) string
}

type report struct {
	cache map[string][]StatementItem

	prefix  string
	perPage int
	tmpl    *template.Template
}

// ReportPage is a structure to render  report content the telegram
type ReportPage struct {
	StatementItems      []StatementItem // items per page
	SpentTotal          int             // total for the period
	AmountTotal         int             // total for the period
	CashbackAmountTotal int             // total for the period
	Period              string
}

// NewReport returns a report object.
func NewReport() Report {

	tmpl, err := GetTempate(reportPageTemplate)
	if err != nil {
		log.Fatalf("[template] %s", err)
	}

	return &report{
		prefix:  "r:",
		perPage: 5,
		cache:   map[string][]StatementItem{},
		tmpl:    tmpl,
	}
}

func (r *report) getCacheKay(update tgbotapi.Update) string {
	var key string

	var fromID int
	var chatID int64

	if update.Message != nil {
		key = update.Message.Text

		fromID = update.Message.From.ID
		chatID = update.Message.Chat.ID
	} else {
		data := r.callbackQueryDataParser(update.CallbackQuery.Data)
		key = data.Period

		fromID = update.CallbackQuery.Message.ReplyToMessage.From.ID
		chatID = update.CallbackQuery.Message.ReplyToMessage.Chat.ID
	}

	return fmt.Sprintf(
		"%s-report-%d-%d",
		key,
		chatID, fromID,
	)
}

func (r *report) SetGridData(update tgbotapi.Update, items []StatementItem) {
	r.cache[r.getCacheKay(update)] = items
}

func (r report) IsExistGridData(update tgbotapi.Update) bool {
	_, ok := r.cache[r.getCacheKay(update)]
	return ok
}

func (r *report) GetReportGrid(update tgbotapi.Update) tgbotapi.MessageConfig {
	items := r.cache[r.getCacheKay(update)]

	var tpl bytes.Buffer
	err := r.tmpl.Execute(&tpl, r.buildReportPage(items, 1, r.perPage))
	if err != nil {
		log.Printf("[processing] template execute error %s", err)
		return tgbotapi.MessageConfig{}
	}
	message := tpl.String()

	inlineKeyboardMarkup := tgbotapi.NewInlineKeyboardMarkup(getPaginateButtons(len(items), 1, r.perPage, r.callbackQueryDataBulder(pageData{
		Page:   1,
		Period: update.Message.Text,
		ChatID: update.Message.Chat.ID,
		FromID: update.Message.From.ID,
	})))

	messageConfig := tgbotapi.MessageConfig{}
	messageConfig.Text = message
	messageConfig.ChatID = update.Message.Chat.ID
	messageConfig.ReplyToMessageID = update.Message.MessageID
	messageConfig.ReplyMarkup = inlineKeyboardMarkup

	return messageConfig
}

func (r report) GetUpdatedReportGrid(update tgbotapi.Update) (tgbotapi.EditMessageTextConfig, error) {
	items := r.cache[r.getCacheKay(update)]
	data := r.callbackQueryDataParser(update.CallbackQuery.Data)

	var tpl bytes.Buffer
	err := r.tmpl.Execute(&tpl, r.buildReportPage(items, data.Page, r.perPage))
	if err != nil {
		log.Printf("[processing] template execute error %s", err)
		return tgbotapi.EditMessageTextConfig{}, err
	}
	message := tpl.String()

	inlineKeyboardMarkup := tgbotapi.NewInlineKeyboardMarkup(getPaginateButtons(len(items), data.Page, r.perPage, r.callbackQueryDataBulder(data)))

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

	if page == 1 {
		items = items[:limit]
	} else if totalPages == page {
		items = items[(totalPages-1)*limit:]
	} else {
		items = items[(page-1)*limit : page*limit]
	}

	return ReportPage{
		StatementItems:      items,
		AmountTotal:         amountTotal,
		SpentTotal:          spentTotal,
		CashbackAmountTotal: cashbackAmountTotal,
	}
}

type pageData struct {
	Page   int
	FromID int
	ChatID int64
	Period string
}

func (r report) callbackQueryDataParser(data string) pageData {
	// prefix + Period + FromID + ChatID + page
	// example: r:1:12321324:312234234:1
	arr := strings.Split(data, ":")

	// not checking errors because it always will be correct numbers
	period := arr[1]
	fromID, _ := strconv.Atoi(arr[2])
	chatID, _ := strconv.ParseInt(arr[3], 10, 64)
	page, _ := strconv.Atoi(arr[4])

	return pageData{
		Page:   page,
		Period: period,
		FromID: fromID,
		ChatID: chatID,
	}
}

func (r report) callbackQueryDataBulder(data pageData) string {
	// prefix + Period + FromID + ChatID + page
	// example: r:1:12321324:312234234:1

	return fmt.Sprintf("%s%s:%d:%d:",
		r.prefix,
		data.Period,
		data.FromID,
		data.ChatID,
		//data.Page,
	)
}

func (r report) IsReportGridPageCommand(update tgbotapi.Update) bool {
	data := update.CallbackQuery.Data
	return strings.HasPrefix(data, r.prefix)
}

func (r report) IsReportGridCommand(update tgbotapi.Update) bool {
	_, ok := reportCommant[update.Message.Text]
	return ok
}

func (r report) GetKeyboardMessageConfig(update tgbotapi.Update) tgbotapi.MessageConfig {
	custom := []tgbotapi.KeyboardButton{
		tgbotapi.KeyboardButton{
			Text: "Today",
		},
		tgbotapi.KeyboardButton{
			Text: "This week",
		},
		tgbotapi.KeyboardButton{
			Text: "Last week",
		},
		tgbotapi.KeyboardButton{
			Text: "This month",
		},
		tgbotapi.KeyboardButton{
			Text: "Last month",
		},
	}
	months := []tgbotapi.KeyboardButton{
		tgbotapi.KeyboardButton{
			Text: "January",
		},
		tgbotapi.KeyboardButton{
			Text: "February",
		},
		tgbotapi.KeyboardButton{
			Text: "March",
		},
		tgbotapi.KeyboardButton{
			Text: "April",
		},
		tgbotapi.KeyboardButton{
			Text: "May",
		},
		tgbotapi.KeyboardButton{
			Text: "June",
		},
	}
	months2 := []tgbotapi.KeyboardButton{
		tgbotapi.KeyboardButton{
			Text: "July",
		},
		tgbotapi.KeyboardButton{
			Text: "August",
		},
		tgbotapi.KeyboardButton{
			Text: "September",
		},
		tgbotapi.KeyboardButton{
			Text: "October",
		},
		tgbotapi.KeyboardButton{
			Text: "November",
		},
		tgbotapi.KeyboardButton{
			Text: "December",
		},
	}

	month := time.Now().Month()
	if int(month) < len(months) {
		months = months[:month]
	}
	if int(month) > len(months2) {
		months2 = months2[:month-6]
	}

	replyKeyboard := tgbotapi.NewReplyKeyboard(custom, months, months2)
	replyKeyboard.OneTimeKeyboard = true

	messageConfig := tgbotapi.MessageConfig{}
	messageConfig.Text = "Selecte a month"
	messageConfig.ChatID = update.Message.Chat.ID
	messageConfig.ReplyToMessageID = update.Message.MessageID
	messageConfig.ReplyMarkup = replyKeyboard

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
