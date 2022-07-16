package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api"
	"github.com/rs/zerolog/log"

	"github.com/snabb/isoweek"
	"golang.org/x/exp/constraints"
)

func getPaginateButtons(total int, page int, limit int, data string) []tgbotapi.InlineKeyboardButton {
	buttons := []tgbotapi.InlineKeyboardButton{}

	totalPages := int(total / limit)
	if total%limit != 0 {
		totalPages++
	}

	if totalPages > 1 {
		pages := 4
		if totalPages <= 4 {
			pages = totalPages
		}

		start := 1
		if page > 3 && page == totalPages {
			start = page - 3
			pages = page

			pageCallbackData := fmt.Sprintf("%s1", data)
			buttons = append(buttons, tgbotapi.InlineKeyboardButton{
				Text:         "«1",
				CallbackData: &pageCallbackData,
			})
		} else if page > 3 && page > totalPages-2 {
			start = page - 2
			pages = page

			pageCallbackData := fmt.Sprintf("%s1", data)
			buttons = append(buttons, tgbotapi.InlineKeyboardButton{
				Text:         "«1",
				CallbackData: &pageCallbackData,
			})
		} else if page > 3 {
			start = page - 1
			pages = page + 1

			pageCallbackData := fmt.Sprintf("%s1", data)
			buttons = append(buttons, tgbotapi.InlineKeyboardButton{
				Text:         "«1",
				CallbackData: &pageCallbackData,
			})
		}

		for i := start; i <= pages; i++ {
			// current
			if i == page {
				pageCallbackData := fmt.Sprintf("%s%d", data, i)
				buttons = append(buttons, tgbotapi.InlineKeyboardButton{
					Text:         fmt.Sprintf("·%d·", i),
					CallbackData: &pageCallbackData,
				})
				// page 2 with ‹
			} else if page > 3 && i == start {
				pageCallbackData := fmt.Sprintf("%s%d", data, i)
				buttons = append(buttons, tgbotapi.InlineKeyboardButton{
					Text:         fmt.Sprintf("‹%d", i),
					CallbackData: &pageCallbackData,
				})

				// page 3 with ›
			} else if i == pages && totalPages > 4 {
				pageCallbackData := fmt.Sprintf("%s%d", data, i)
				buttons = append(buttons, tgbotapi.InlineKeyboardButton{
					Text:         fmt.Sprintf("%d›", i),
					CallbackData: &pageCallbackData,
				})
			} else {
				pageCallbackData := fmt.Sprintf("%s%d", data, i)
				buttons = append(buttons, tgbotapi.InlineKeyboardButton{
					Text:         fmt.Sprintf("%d", i),
					CallbackData: &pageCallbackData,
				})
			}
		}
	}

	if page != totalPages && page > totalPages-2 && totalPages > 4 {
		pageCallbackData := fmt.Sprintf("%s%d", data, totalPages)
		buttons = append(buttons, tgbotapi.InlineKeyboardButton{
			Text:         fmt.Sprintf("%d", totalPages),
			CallbackData: &pageCallbackData,
		})
	} else if page != totalPages && totalPages > 4 {
		pageCallbackData := fmt.Sprintf("%s%d", data, totalPages)
		buttons = append(buttons, tgbotapi.InlineKeyboardButton{
			Text:         fmt.Sprintf("%d»", totalPages),
			CallbackData: &pageCallbackData,
		})
	}

	return buttons
}

func abs[T constraints.Integer | constraints.Float](n T) T {
	if n < 0 {
		return -n
	}
	return n
}

func getTimeRangeByPeriod(period string) (int64, int64, error) {
	var from, to int64

	period, ok := reportCommant[period]
	if !ok {
		return from, to, errors.New("incorrect period")
	}

	kiev, err := time.LoadLocation("Europe/Kiev")
	if err != nil {
		return from, to, err
	}

	now := time.Now().In(kiev)
	year, month, day := now.Date()

	switch period {
	case "Today":
		startOfDay := time.Date(year, month, day, 0, 0, 0, 0, now.Location())
		from = startOfDay.Unix()
	case "This week":
		_, week := now.ISOWeek()
		startOfWeek := isoweek.StartTime(year, week, now.Location())
		from = startOfWeek.Unix()
	case "Last week":
		_, week := now.ISOWeek()
		startOfLastWeek := isoweek.StartTime(year, week-1, now.Location())
		from = startOfLastWeek.Unix()

		endOfLastWeek := isoweek.StartTime(year, week, now.Location())
		to = endOfLastWeek.Unix()
	case "This month":
		startOfMonth := time.Date(year, month, 1, 0, 0, 0, 0, now.Location())
		from = startOfMonth.Unix()
	case "Last month":
		startOfMonth := time.Date(year, month-1, 1, 0, 0, 0, 0, now.Location())
		from = startOfMonth.Unix()

		endOfMonth := time.Date(year, month, 1, 0, 0, 0, 0, now.Location())
		to = endOfMonth.Unix()
	default:
		numberOfMonth, err := strconv.Atoi(period)
		if err != nil {
			return from, to, err
		}

		startOfMonth := time.Date(year, time.Month(numberOfMonth), 1, 0, 0, 0, 0, now.Location())
		from = startOfMonth.Unix()

		endOfMonth := time.Date(year, time.Month(numberOfMonth)+1, 1, 0, 0, 0, 0, now.Location())
		to = endOfMonth.Unix()
	}

	return from, to, nil
}

type pageData struct {
	Page     int
	Account  string
	FromID   int
	ChatID   int64
	Period   string
	ClientID uint32
}

func callbackQueryDataParser(data string) pageData {
	// prefix + Period + FromID + ChatID + clientID + page
	// example: r:1:12321324:312234234:23423432:1
	arr := strings.Split(data, ":")

	// not checking errors because it always will be correct numbers
	period := arr[1]
	clientID, _ := strconv.ParseUint(arr[4], 10, 32)
	account := arr[5]
	page, _ := strconv.Atoi(arr[6])

	return pageData{
		Page:     page,
		Period:   period,
		ClientID: uint32(clientID),
		Account:  account,
	}
}

func callbackQueryDataBuilder(prefix string, data pageData) string {
	// prefix + Period + FromID + ChatID + clientID + page
	// example: r:1:12321324:312234234:23423432:1

	return fmt.Sprintf("%s:%s:%d:%d:%d:%s:",
		prefix,
		data.Period,
		0, // data.FromID,
		0, // data.ChatID,
		data.ClientID,
		data.Account,
		//data.Page,
	)
}

// IsURL is a url validate function
func IsURL(str string) bool {
	u, err := url.Parse(str)
	return err == nil && u.Scheme != "" && u.Host != ""
}

func DoRequest[D any](data D, req *http.Request) (D, error) {
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Error().Err(err).Msg("[DoRequest] request")
		return data, err
	}

	defer res.Body.Close()
	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		log.Error().Err(err).Msg("[DoRequest] read body")
		return data, err
	}

	if err := json.Unmarshal(body, &data); err != nil {
		log.Error().Err(err).Msg("[DoRequest] unmarshal")
		return data, err
	}

	log.Debug().Msgf("[DoRequest] responce %s", string(body))
	return data, nil
}
