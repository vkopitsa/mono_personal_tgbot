package main

import (
	"bytes"
	"context"
	"fmt"

	"github.com/rs/zerolog/log"
	"github.com/samber/lo"
)

type ScheduleReportData struct {
	ClientInfo     ClientInfo
	StatementItems []StatementItem
	Sum            int
	CashbackSum    int
	Count          int
	Currencies     Currencies
}

func (s *ScheduleReportData) IsEmpty() bool {
	return s.StatementItems == nil || len(s.StatementItems) == 0
}

func (s *ScheduleReportData) Prepare() {
	if s.IsEmpty() {
		return
	}

	_, ignoreAll := s.filterAndReduceStatements()
	statements := s.filterStatements(ignoreAll)

	s.Count = len(statements)
	s.Sum = s.calculateSum(statements)
	s.CashbackSum = s.calculateCashbackSum(statements)
}

func (s *ScheduleReportData) filterAndReduceStatements() (map[string]string, map[string]bool) {
	fromStatements := s.filter4829Statements()
	fromStatementsMap := s.reduceStatements(fromStatements)

	ignoreAll := map[string]bool{}
	_ = s.filterOutFromAccounts(fromStatementsMap, ignoreAll)

	return fromStatementsMap, ignoreAll
}

func (s *ScheduleReportData) filter4829Statements() []StatementItem {
	return lo.Filter[StatementItem](s.StatementItems, func(item StatementItem, index int) bool {
		return item.Mcc == 4829 && item.OriginalMcc == 4829 && item.Amount < 0 && item.OperationAmount < 0 && item.Amount != item.OperationAmount
	})
}

func (s *ScheduleReportData) reduceStatements(fromStatements []StatementItem) map[string]string {
	return lo.Reduce[StatementItem, map[string]string](fromStatements, func(agg map[string]string, item StatementItem, index int) map[string]string {
		agg[fmt.Sprintf("%d %d %d %d", item.Mcc, item.OriginalMcc, -item.Amount, -item.OperationAmount)] = item.ID
		return agg
	}, map[string]string{})
}

func (s *ScheduleReportData) filterOutFromAccounts(fromStatementsMap map[string]string, ignoreAll map[string]bool) []StatementItem {
	return lo.Filter[StatementItem](s.StatementItems, func(item StatementItem, index int) bool {
		key := fmt.Sprintf("%d %d %d %d", item.Mcc, item.OriginalMcc, item.OperationAmount, item.Amount)

		id, ok := fromStatementsMap[key]
		if ok {
			ignoreAll[item.ID] = true
			ignoreAll[id] = true
		}
		return !ok
	})
}

func (s *ScheduleReportData) filterStatements(ignoreAll map[string]bool) []StatementItem {
	return lo.Filter[StatementItem](s.StatementItems, func(item StatementItem, index int) bool {
		_, ok := ignoreAll[item.ID]
		return !ok
	})
}

func (s *ScheduleReportData) calculateSum(statements []StatementItem) int {
	return lo.Reduce[StatementItem, int](statements, func(agg int, item StatementItem, index int) int {
		if item.Amount < 0 {
			agg += s.calculateAmount(item)
		}
		return agg
	}, 0)
}

func (s *ScheduleReportData) calculateAmount(item StatementItem) int {
	if item.Amount == item.OperationAmount && item.CurrencyCode != 980 {
		return s.calculateNonUAHAmount(item)
	} else if item.Amount != item.OperationAmount && item.CurrencyCode == 980 {
		return -item.OperationAmount
	}
	return -item.Amount
}

func (s *ScheduleReportData) calculateNonUAHAmount(item StatementItem) int {
	currency, ok := lo.Find[Currency](s.Currencies, func(citem Currency) bool {
		return citem.CurrencyCodeA == item.CurrencyCode && citem.CurrencyCodeB == 980
	})
	if ok {
		return -item.Amount * (currency.CurrencyCodeA * 100)
	}
	return 0
}

func (s *ScheduleReportData) calculateCashbackSum(statements []StatementItem) int {
	return lo.Reduce[StatementItem, int](statements, func(agg int, item StatementItem, index int) int {
		agg = agg + item.CashbackAmount
		return agg
	}, 0)
}

func (b *bot) ScheduleReport(ctx context.Context) (int, error) {
	if len(b.clients) == 0 {
		return 0, nil
	}

	tmpl, err := GetTempate(scheduleReportTemplate)
	if err != nil {
		log.Fatal().Err(err).Msg("[template] error")
	}

	currencies, err := b.mono.GetCurrencies()
	if err != nil {
		log.Err(err)
	}

	for _, client := range b.clients {
		scheduleReportData := ScheduleReportData{
			StatementItems: []StatementItem{},
			Currencies:     currencies,
		}

		info, err := client.GetInfo()
		if err != nil {
			log.Err(err)
			continue
		}

		scheduleReportData.ClientInfo = info

		for _, account := range info.Accounts {
			items, err := client.GetStatement("Today", account.ID)
			if err != nil {
				log.Error().Err(err).Msg("[monobank] report, get statements")
				continue
			}
			scheduleReportData.StatementItems = append(scheduleReportData.StatementItems, items...)
		}

		if scheduleReportData.IsEmpty() {
			log.Info().Msg("[monobank] schedule report")
			continue
		}

		scheduleReportData.Prepare()

		if scheduleReportData.Count == 0 || scheduleReportData.Sum == 0 {
			log.Info().Msg("[monobank] schedule report, empty after filter")
			continue
		}

		var tpl bytes.Buffer
		err = tmpl.Execute(&tpl, scheduleReportData)
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
	}

	return 0, nil
}
