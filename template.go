package main

import (
	"fmt"
	"html/template"
)

// Statement template, use the StatementItem structure
var statementTemplate = `{{ getIcon . }} {{ normalizePrice .Amount }}{{if .CashbackAmount }}, Кешбек: {{ normalizePrice .CashbackAmount }}{{end}}
{{ .Description }}{{if .Comment }}
Коментар: {{ .Comment }}{{end}}
Баланс: {{ normalizePrice .Balance }}`

// Balance template, use the Account structure
var balanceTemplate = `Баланс: {{ normalizePrice .Balance }}`

// Report template, Use the ReportPage structure
var reportPageTemplate = `Витрачено: {{ normalizePrice .SpentTotal }}, Кешбек: {{ normalizePrice .CashbackAmountTotal }}

{{range $item := .StatementItems }}{{ getIcon $item }} {{ normalizePrice $item.Amount }}{{if $item.CashbackAmount }}, Кешбек: {{ normalizePrice $item.CashbackAmount }}{{end}}
{{ $item.Description }}{{if $item.Comment }}
Коментар: {{ $item.Comment }}{{end}}
Баланс: {{ normalizePrice $item.Balance }}

{{end}}`

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
}

// GetTempate is a function to parse template with functions
func GetTempate(templateBody string) (*template.Template, error) {
	return template.New("message").
		Funcs(template.FuncMap{
			"normalizePrice": func(price int) string {
				if price%100 == 0 {
					return fmt.Sprintf("%d₴", price/100)
				}
				return fmt.Sprintf("%.2f₴", float64(price)/100.0)
			},
			"getIcon": func(statementItem StatementItem) string {
				return GetIconByStatementItem(statementItem)
			},
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
		// defoult emoji
		icon = mccIcon
	}

	return icon
}
