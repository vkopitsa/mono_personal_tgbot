package main

import (
	"fmt"
	"html/template"
)

// Use the StatementItem structure
var statementTemplate = `Потрачено: {{ normalizePrice .Amount }} на {{ .Description }}{{if .Comment }}, {{ .Comment }}{{end}}
Cashback: {{ normalizePrice .CashbackAmount }}
Баланс: {{ normalizePrice .Balance }}`

// Use the Account structure
var balanceTemplate = `"Баланс: {{ normalizePrice .Balance }}"`

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
		}).
		Parse(templateBody)
}
