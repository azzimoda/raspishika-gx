package vkbotutil

import "strconv"

// MenuItem is one entry of a paged picker menu.
type MenuItem struct {
	Label   string
	Command string
}

// PageSize bounds the items shown on one page of a picker.
const PageSize = 6

// PageNumber parses a page index, defaulting to the first page.
func PageNumber(text string) int {
	page, err := strconv.Atoi(text)
	if err != nil || page < 0 {
		return 0
	}
	return page
}

// PagedKeyboard builds a keyboard of item rows plus optional navigation and a
// back button, staying within VK's inline keyboard limits even when both
// navigation buttons are present.
func PagedKeyboard(items []MenuItem, page int, pageCommand, backCommand string) *Keyboard {
	pages := (len(items) + PageSize - 1) / PageSize
	if pages == 0 {
		pages = 1
	}
	if page < 0 {
		page = 0
	}
	if page >= pages {
		page = pages - 1
	}
	start, end := page*PageSize, (page+1)*PageSize
	if end > len(items) {
		end = len(items)
	}
	rows := [][]Button{}
	for i := start; i < end; i += 2 {
		row := []Button{TextButton(shortLabel(items[i].Label), items[i].Command)}
		if i+1 < end {
			row = append(row, TextButton(shortLabel(items[i+1].Label), items[i+1].Command))
		}
		rows = append(rows, row)
	}
	var nav []Button
	if page > 0 {
		nav = append(nav, TextButton("← Назад", pageCommand+"\n"+strconv.Itoa(page-1)))
	}
	if page+1 < pages {
		nav = append(nav, TextButton("Далее →", pageCommand+"\n"+strconv.Itoa(page+1)))
	}
	if len(nav) > 0 {
		rows = append(rows, nav)
	}
	rows = append(rows, []Button{TextButton("Назад", backCommand)})
	return &Keyboard{Inline: true, Buttons: rows}
}

func shortLabel(text string) string {
	runes := []rune(text)
	if len(runes) > 40 {
		return string(runes[:39]) + "…"
	}
	return text
}
