package main

import (
	"errors"
	"fmt"
	"strconv"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api"
)

func getRangByCommand(command string) (int64, int64, error) {
	item, ok := reportCommant[command]
	if !ok {
		return 0, 0, errors.New("Command does not found")
	}

	now := time.Now()

	if isManualCommand(item) {
		switch command {
		case "Today":
			return 0, 0, nil
		case "This week":
			return 0, 0, nil
		case "Last week":
			//return 0, 0, nil
			return now.Add(time.Duration(-60*24*7) * time.Minute).Unix(), now.Unix(), nil
		case "This month":
			// return now.AddDate(1, 0, 0).Unix(), now.Unix(), nil
			//return now.Add(time.Duration(-1) * (time.Minute * 60 * 24 * 6))
		case "Last month":
			return now.AddDate(0, -1, 0).Unix(), now.Unix(), nil
			//return now.Add(time.Duration(-1) * time.Month)
		default:
			return 0, 0, errors.New("Manual command does not found")
		}
	} else {

	}

	return 0, 0, nil
}

func isManualCommand(command string) bool {
	switch command {
	case
		"Today",
		"This week",
		"Last week",
		"This month",
		"Last month":
		return true
	}
	return false
}

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
				Text:         fmt.Sprintf("«1"),
				CallbackData: &pageCallbackData,
			})
		} else if page > 3 && page > totalPages-2 {
			start = page - 2
			pages = page

			pageCallbackData := fmt.Sprintf("%s1", data)
			buttons = append(buttons, tgbotapi.InlineKeyboardButton{
				Text:         fmt.Sprintf("«1"),
				CallbackData: &pageCallbackData,
			})
		} else if page > 3 {
			start = page - 1
			pages = page + 1

			pageCallbackData := fmt.Sprintf("%s1", data)
			buttons = append(buttons, tgbotapi.InlineKeyboardButton{
				Text:         fmt.Sprintf("«1"),
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

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

func getTimeRangeByPeriod(period string) (int64, int64, error) {
	var from, to int64

	period, ok := reportCommant[period]
	if !ok {
		return from, to, errors.New("Incorrect period")
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
		break
	case "This week":
		_, week := now.ISOWeek()
		startOfWeek := firstDayOfISOWeek(year, week, now.Location())
		from = startOfWeek.Unix()
		break
	case "Last week":
		_, week := now.ISOWeek()
		startOfLastWeek := firstDayOfISOWeek(year, week-1, now.Location())
		from = startOfLastWeek.Unix()

		endOfLastWeek := firstDayOfISOWeek(year, week, now.Location())
		to = endOfLastWeek.Unix()
		break
	case "This month":
		startOfMonth := time.Date(year, month, 1, 0, 0, 0, 0, now.Location())
		from = startOfMonth.Unix()

		break
	case "Last month":
		startOfMonth := time.Date(year, month-1, 1, 0, 0, 0, 0, now.Location())
		from = startOfMonth.Unix()

		endOfMonth := time.Date(year, month, 1, 0, 0, 0, 0, now.Location())
		to = endOfMonth.Unix()
		break
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

func firstDayOfISOWeek(year int, week int, timezone *time.Location) time.Time {
	date := time.Date(year, 0, 0, 0, 0, 0, 0, timezone)
	isoYear, isoWeek := date.ISOWeek()
	for date.Weekday() != time.Monday { // iterate back to Monday
		date = date.AddDate(0, 0, -1)
		isoYear, isoWeek = date.ISOWeek()
	}
	for isoYear < year { // iterate forward to the first day of the first week
		date = date.AddDate(0, 0, 1)
		isoYear, isoWeek = date.ISOWeek()
	}
	for isoWeek < week { // iterate forward to the first day of the given week
		date = date.AddDate(0, 0, 1)
		isoYear, isoWeek = date.ISOWeek()
	}
	return date
}