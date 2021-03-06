package main

import "testing"

func TestGetPaginateButtonsPage1(t *testing.T) {
	total := 57
	limit := 5
	page := 1
	buttons := getPaginateButtons(total, page, limit, "")

	if len(buttons) != 5 {
		t.Error("Expected 5, got ", len(buttons))
	}

	var tests = []struct {
		button   int
		expected string
	}{
		{0, "·1·"},
		{1, "2"},
		{2, "3"},
		{3, "4›"},
		{4, "12»"},
	}

	for _, test := range tests {
		if buttons[test.button].Text != test.expected {
			t.Error(
				"button", test.button,
				"expected", test.expected,
				"got", buttons[test.button].Text,
			)
		}
	}
}

func TestGetPaginateButtonsPage1Pages3(t *testing.T) {
	total := 15
	limit := 5
	page := 1
	buttons := getPaginateButtons(total, page, limit, "")

	if len(buttons) != 3 {
		t.Error("Expected 3, got ", len(buttons))
	}

	var tests = []struct {
		button   int
		expected string
	}{
		{0, "·1·"},
		{1, "2"},
		{2, "3"},
	}

	for _, test := range tests {
		if buttons[test.button].Text != test.expected {
			t.Error(
				"button", test.button,
				"expected", test.expected,
				"got", buttons[test.button].Text,
			)
		}
	}
}

func TestGetPaginateButtonsPage2(t *testing.T) {
	total := 57
	limit := 5
	page := 2
	buttons := getPaginateButtons(total, page, limit, "")
	if len(buttons) != 5 {
		t.Error("Expected 5, got ", len(buttons))
	}

	var tests = []struct {
		button   int
		expected string
	}{
		{0, "1"},
		{1, "·2·"},
		{2, "3"},
		{3, "4›"},
		{4, "12»"},
	}

	for _, test := range tests {
		if buttons[test.button].Text != test.expected {
			t.Error(
				"button", test.button,
				"expected", test.expected,
				"got", buttons[test.button].Text,
			)
		}
	}
}

func TestGetPaginateButtonsPage7(t *testing.T) {
	total := 55
	limit := 5
	page := 7
	buttons := getPaginateButtons(total, page, limit, "")
	if len(buttons) != 5 {
		t.Error("Expected 5, got ", len(buttons))
	}

	var tests = []struct {
		button   int
		expected string
	}{
		{0, "«1"},
		{1, "‹6"},
		{2, "·7·"},
		{3, "8›"},
		{4, "11»"},
	}

	for _, test := range tests {
		if buttons[test.button].Text != test.expected {
			t.Error(
				"button", test.button,
				"expected", test.expected,
				"got", buttons[test.button].Text,
			)
		}
	}
}

func TestGetPaginateButtonsPage4(t *testing.T) {
	total := 55
	limit := 5
	page := 4
	buttons := getPaginateButtons(total, page, limit, "")
	if len(buttons) != 5 {
		t.Error("Expected 5, got ", len(buttons))
	}

	var tests = []struct {
		button   int
		expected string
	}{
		{0, "«1"},
		{1, "‹3"},
		{2, "·4·"},
		{3, "5›"},
		{4, "11»"},
	}

	for _, test := range tests {
		if buttons[test.button].Text != test.expected {
			t.Error(
				"button", test.button,
				"expected", test.expected,
				"got", buttons[test.button].Text,
			)
		}
	}
}

func TestGetPaginateButtonsLastPage(t *testing.T) {
	total := 55
	limit := 5
	page := 11
	buttons := getPaginateButtons(total, page, limit, "")
	if len(buttons) != 5 {
		t.Error("Expected 5, got ", len(buttons))
	}

	var tests = []struct {
		button   int
		expected string
	}{
		{0, "«1"},
		{1, "‹8"},
		{2, "9"},
		{3, "10"},
		{4, "·11·"},
	}

	for _, test := range tests {
		if buttons[test.button].Text != test.expected {
			t.Error(
				"button", test.button,
				"expected", test.expected,
				"got", buttons[test.button].Text,
			)
		}
	}
}

func TestGetPaginateButtonsPreLastPage(t *testing.T) {
	total := 55
	limit := 5
	page := 10
	buttons := getPaginateButtons(total, page, limit, "")
	if len(buttons) != 5 {
		t.Error("Expected 5, got ", len(buttons))
	}

	var tests = []struct {
		button   int
		expected string
	}{
		{0, "«1"},
		{1, "‹8"},
		{2, "9"},
		{3, "·10·"},
		{4, "11"},
	}

	for _, test := range tests {
		if buttons[test.button].Text != test.expected {
			t.Error(
				"button", test.button,
				"expected", test.expected,
				"got", buttons[test.button].Text,
			)
		}
	}
}

func TestGetPaginateButtonsCouplePage1(t *testing.T) {
	total := 10
	limit := 5
	page := 1
	buttons := getPaginateButtons(total, page, limit, "")
	if len(buttons) != 2 {
		t.Error("Expected 2, got ", len(buttons))
	}

	var tests = []struct {
		button   int
		expected string
	}{
		{0, "·1·"},
		{1, "2"},
	}

	for _, test := range tests {
		if buttons[test.button].Text != test.expected {
			t.Error(
				"button", test.button,
				"expected", test.expected,
				"got", buttons[test.button].Text,
			)
		}
	}
}

func TestGetPaginateButtonsCouplePage2(t *testing.T) {
	total := 10
	limit := 5
	page := 2
	buttons := getPaginateButtons(total, page, limit, "")
	if len(buttons) != 2 {
		t.Error("Expected 2, got ", len(buttons))
	}

	var tests = []struct {
		button   int
		expected string
	}{
		{0, "1"},
		{1, "·2·"},
	}

	for _, test := range tests {
		if buttons[test.button].Text != test.expected {
			t.Error(
				"button", test.button,
				"expected", test.expected,
				"got", buttons[test.button].Text,
			)
		}
	}
}
