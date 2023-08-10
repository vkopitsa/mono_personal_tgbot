package main

import (
	"fmt"
	"html"
	"html/template"
)

// Statement template, use the StatementItem structure and Name field
var statementTemplate = ` {{ .Name }}
{{ getIcon .StatementItem }} {{ normalizePrice .StatementItem.Amount }}{{ getCurrencySymbol .Account.CurrencyCode }}{{ if ne .StatementItem.Amount .StatementItem.OperationAmount }} ({{ normalizePrice .StatementItem.OperationAmount }}{{ getCurrencySymbol .StatementItem.CurrencyCode }}){{end}}{{if .StatementItem.CashbackAmount }}, Кешбек: {{ normalizePrice .StatementItem.CashbackAmount }}{{ getCurrencySymbol .StatementItem.CurrencyCode }}{{end}}
{{ unescapeString .StatementItem.Description }}{{if .StatementItem.Comment }}
Коментар: {{ unescapeString .StatementItem.Comment }}{{end}}
Баланс: {{ normalizePrice .StatementItem.Balance }}{{ getCurrencySymbol .Account.CurrencyCode }}`

// Balance template, use the Account structure
var balanceTemplate = `{{ .Name }}

{{range $item := .Accounts }}- {{ .Type }}
Баланс: {{ normalizePrice $item.Balance }}{{ getCurrencySymbol $item.CurrencyCode }}
{{end}}`

// Report template, Use the ReportPage structure
var reportPageTemplate = `{{ $symbol := getCurrencySymbol .CurrencyCode }}Витрачено: {{ normalizePrice .SpentTotal }}{{ $symbol }}, Кешбек: {{ normalizePrice .CashbackAmountTotal }}{{ $symbol }}

{{range $item := .StatementItems }}{{ getIcon $item }} {{ normalizePrice $item.Amount }}{{ $symbol }} {{ if ne $item.Amount $item.OperationAmount }} ({{ normalizePrice $item.OperationAmount }}{{ getCurrencySymbol $item.CurrencyCode }}){{end}}{{if $item.CashbackAmount }}, Кешбек: {{ normalizePrice $item.CashbackAmount }}{{ getCurrencySymbol $item.CurrencyCode }}{{end}}
{{ unescapeString $item.Description }}{{if $item.Comment }}
Коментар: {{ unescapeString $item.Comment }}{{end}}
Баланс: {{ normalizePrice $item.Balance }}{{ $symbol }}

{{end}}`

// Schedule Report template, Use the ScheduleReportData structure
var scheduleReportTemplate = `Щоденна статистика рахунків, {{ .ClientInfo.Name }}

Витрачено: {{ normalizePrice .Sum }} UAH
Кешбек: {{ normalizePrice .CashbackSum }} UAH
Транзакцій: {{ .Count }}`

// WebHook template, use the ClientInfo structure
var webhookTemplate = `Вебхук: {{if .WebHookURL }}{{ .WebHookURL }}{{else}} Відсутній {{end}}`

// mccIconMap is map to help converting MMC code to emoji
// see https://mcc.in.ua/ to explain a code
var mccIconMap = map[int]string{
	5411: "🍞",
	5814: "🍔",
	8999: "🏢",
	5499: "🛍",
	5651: "👕",
	5655: "🥊",
	6011: "🏧",
	4814: "📱",
	7399: "💼",
	2842: "🔧",
	5977: "💋",
	5912: "💊",
}

// currencySymbolMap is map to help converting currency code to Symbol
var currencySymbolMap = map[int]string{
	980: "₴",
	840: "$",
	978: "€",
	985: "zł",
	203: "Kč",
}

// GetTempate is a function to parse template with functions
func GetTempate(templateBody string) (*template.Template, error) {
	return template.New("message").
		Funcs(template.FuncMap{
			"normalizePrice":    NormalizePrice,
			"getIcon":           GetIconByStatementItem,
			"getCurrencySymbol": GetCurrencySymbol,
			"unescapeString":    html.UnescapeString,
		}).
		Parse(templateBody)
}

// GetIconByStatementItem is a function get emoji/icons by MCC code
func GetIconByStatementItem(statementItem StatementItem) string {
	// defoult emoji
	icon := "🛒"

	// Money transfers
	if statementItem.Mcc == 4829 {
		if statementItem.Amount > 0 {
			icon = "👉💳"
		} else {
			icon = "👈💳"
		}
	}

	mccIcon, ok := mccIconMap[statementItem.Mcc]
	if ok {
		icon = mccIcon
	}

	return icon
}

// GetCurrencySymbol is a function get currency symbol by code
func GetCurrencySymbol(currencyCode int) string {
	symbol := ""

	currencySymbol, ok := currencySymbolMap[currencyCode]
	if ok {
		symbol = currencySymbol
	}

	return symbol
}

// NormalizePrice is a function to normalize price
func NormalizePrice(price int) string {
	if price%100 == 0 {
		return fmt.Sprintf("%d", price/100)
	}
	return fmt.Sprintf("%.2f", float64(price)/100.0)
}
