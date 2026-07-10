package scraper

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/mxschmitt/playwright-go"
	"github.com/rs/zerolog/log"
	"golang.org/x/net/html"

	"github.com/azzimoda/raspishika-gx/internal/model"
)

var ErrParserPanicked = errors.New("parser panicked")

func parseDepartmentGroups(p playwright.Page, department *model.Department) ([]model.Group, error) {
	log.Trace().Msg("Navigating to department page")
	if _, err := p.Goto(department.URL.String()); err != nil {
		return nil, fmt.Errorf("failed to navigate to department page: %w", err)
	}

	frameLocator := p.FrameLocator("div.com-content-article__body iframe")
	if err := frameLocator.Locator("#groups").WaitFor(playwright.LocatorWaitForOptions{
		Timeout: playwright.Float(60_000),
	}); err != nil {
		return nil, fmt.Errorf("failed to wait for groups iframe: %w", err)
	}

	options, err := frameLocator.Locator("#groups option").EvaluateAll(
		`els => els.map(el => ({ text: el.textContent.trim(), value: el.value, sid: el.getAttribute("sid"), year: el.getAttribute("year") }))`)
	if err != nil {
		return nil, fmt.Errorf("failed to get groups options: %w", err)
	}

	var groups []model.Group
	for _, opt := range options.([]any) {
		opt := opt.(map[string]any)
		if !(validateOptionValue(opt["value"]) &&
			validateOptionValue(opt["text"]) &&
			validateOptionValue(opt["sid"]) &&
			validateOptionValue(opt["year"])) {
			log.Trace().Msg("Option is invalid")

			continue
		}

		year, err := strconv.ParseInt(opt["year"].(string), 10, 64)
		if err != nil {
			continue
		}

		groups = append(groups, model.Group{
			GroupID:        model.GroupID(opt["value"].(string)),
			DepartmentID:   model.DepartmentID(opt["sid"].(string)),
			GroupName:      model.GroupName(opt["text"].(string)),
			Year:           model.Year(year),
			DepartmentName: department.Name,
		})
	}
	return groups, nil
}

func parseSchedule(sourceHTML string, conf model.ScheduleConfig) (schedule *model.RawSchedule, err error) {
	defer func() {
		if r := recover(); r != nil {
			log.Error().Msg("Parser panicked!")
			err = fmt.Errorf("%w: %v", ErrParserPanicked, r)
		}
	}()

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(sourceHTML))
	if err != nil {
		return nil, fmt.Errorf("failed to parse HTML: %w", err)
	}

	table := doc.Find("table#main_table")
	if table.Length() == 0 {
		return nil, fmt.Errorf("table element not found")
	}

	var headers []map[string]string
	table.Find("tr").First().Find("td").Slice(2, goquery.ToEnd).Each(func(i int, s *goquery.Selection) {
		var parts []string
		for _, node := range s.Nodes {
			for c := node.FirstChild; c != nil; c = c.NextSibling {
				if c.Type == html.TextNode {
					text := strings.TrimSpace(c.Data)
					if text != "" {
						parts = append(parts, text)
					}
				}
			}
		}

		for len(parts) < 3 {
			parts = append(parts, "")
		}

		headers = append(headers, map[string]string{"date": parts[0], "weekday": parts[1], "week_kind": parts[2]})
	})

	var rows []model.RawScheduleRow
	table.Find("tr.para_num:not(:first-child)").Each(func(i int, s *goquery.Selection) {
		rows = append(rows, parseScheduleRow(&conf, headers, s))
	})
	return &model.RawSchedule{Config: conf, Rows: rows}, nil
}

func parseScheduleRow(config *model.ScheduleConfig, headers []map[string]string, rowSelection *goquery.Selection) model.RawScheduleRow {
	numberStr := rowSelection.Find("td:first-child").First().Text()
	time_range := rowSelection.Find("td:nth-child(2)")

	number, err := strconv.Atoi(numberStr)
	if err != nil {
		panic(err)
	}

	row := model.RawScheduleRow{
		Number:    number,
		TimeRange: model.TimeRange(time_range.First().Text()),
		Days:      []model.RawScheduleDay{},
	}
	rowSelection.Find("td:nth-child(n+3)").Each(func(i int, daySelection *goquery.Selection) {
		row.Days = append(row.Days, parseScheduleDay(config, headers[i], daySelection))
	})

	return row
}

func parseScheduleDay(
	config *model.ScheduleConfig,
	header map[string]string,
	daySelection *goquery.Selection,
) model.RawScheduleDay {
	day := model.RawScheduleDay{
		Date:     model.Date(header["date"]),
		WeekDay:  model.Weekday(header["weekday"]),
		WeekKind: model.WeekKind(header["week_kind"]),
		Pair:     model.Pair{},
	}

	day.Pair.Replaced = daySelection.Find("table").HasClass("zamena")
	day.Pair.Kind = detectPairKind(daySelection)

	switch day.Pair.Kind {
	case model.PairKindSubject:
		parseDisciplinePair(config, daySelection, &day.Pair)
	case model.PairKindExam, model.PairKindConsultation:
		parseExamConsultationPair(daySelection, &day.Pair)
	case model.PairKindEmpty:
		// Nothing
	default:
		parseOtherPair(daySelection, &day.Pair)
	}

	return day
}

func parseDisciplinePair(config *model.ScheduleConfig, daySelection *goquery.Selection, pair *model.Pair) {
	// log.Trace().Str("text", daySelection.Text()).Msg("teacher found")
	teacher := daySelection.Find(".prep").Text()
	pair.Teacher = &teacher
	classroom := daySelection.Find(".cabs").Text()
	pair.Classroom = classroom
	subgroupSelection := daySelection.Find(".podgrupp")
	if subgroupSelection != nil {
		pair.Subgroup = subgroupSelection.Text()
	}

	if config.Group != nil {
		discipline := daySelection.Find(".disc").Text()
		pair.Discipline = discipline
	} else {
		var parts []string
		for _, node := range daySelection.Find(".disc").Nodes {
			for c := node.FirstChild; c != nil; c = c.NextSibling {
				if c.Type == html.TextNode {
					text := strings.TrimSpace(c.Data)
					if text != "" {
						parts = append(parts, text)
					}
				} else if c.Type == html.ElementNode && c.Data == "div" {
					parts = append(parts, c.FirstChild.Data)
				}
			}
		}

		for len(parts) < 2 {
			parts = append(parts, "")
		}

		pair.Discipline = parts[0]
		pair.Group = &parts[1]
	}
}

func parseExamConsultationPair(daySelection *goquery.Selection, pair *model.Pair) {
	pair.Title = daySelection.Find(".head_ekz").Text()
	pair.Discipline = daySelection.Find(".disc").Text()
	teacher := daySelection.Find(".prep").Text()
	pair.Teacher = &teacher
	pair.Classroom = daySelection.Find(".cabs").Text()
}

func parseOtherPair(daySelection *goquery.Selection, pair *model.Pair) {
	pair.Label = daySelection.Text()
}

func detectPairKind(daySelection *goquery.Selection) model.PairKind {
	switch {
	case strings.Contains(strings.ToLower(daySelection.Find(".disc").Text()), "снято"):
		return model.PairKindEmpty
	case daySelection.Find(".disc").Text() != "":
		return model.PairKindSubject
	case daySelection.HasClass("head_urok_kanik"):
		return model.PairKindVacation
	case daySelection.HasClass("event"):
		return model.PairKindEvent
	case daySelection.HasClass("head_urok_praktik"):
		return model.PairKindPractice
	case daySelection.HasClass("head_urok_session"):
		return model.PairKindSession
	case daySelection.HasClass("head_urok_iga"):
		return model.PairKindIGA
	case daySelection.HasClass("zachet") || daySelection.HasClass("difzachet") || daySelection.HasClass("ekzamen"):
		return model.PairKindExam
	case daySelection.Find("table.consultation").Length() > 0:
		return model.PairKindConsultation
	default:
		return model.PairKindEmpty
	}
}
