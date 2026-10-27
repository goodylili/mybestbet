package fbref

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"golang.org/x/net/html"
)

type row struct {
	fbrefID string
	cells   map[string]string
}

var (
	playerIDFromHref = regexp.MustCompile(`/players/([0-9a-f]+)/`)
	seasonInCaption  = regexp.MustCompile(`\b(\d{4}(?:-\d{4})?)\b`)
)

func parseTable(page []byte, tableID string) (rows []row, season string, err error) {
	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(page))
	if err != nil {
		return nil, "", fmt.Errorf("parse page: %w", err)
	}
	table, err := locateTable(doc, tableID)
	if err != nil {
		return nil, "", err
	}
	season = seasonInCaption.FindString(table.Find("caption span").First().Text())
	table.Find("tbody tr").Each(func(_ int, tr *goquery.Selection) {
		if tr.HasClass("thead") {
			return
		}
		if r, ok := readRow(tr); ok {
			rows = append(rows, r)
		}
	})
	return rows, season, nil
}

func locateTable(doc *goquery.Document, tableID string) (*goquery.Selection, error) {
	if table := doc.Find("table#" + tableID); table.Length() > 0 {
		return table.First(), nil
	}
	for _, commented := range commentsUnder(doc.Find("#all_" + tableID)) {
		inner, err := goquery.NewDocumentFromReader(strings.NewReader(commented))
		if err != nil {
			return nil, fmt.Errorf("parse commented table %s: %w", tableID, err)
		}
		if table := inner.Find("table#" + tableID); table.Length() > 0 {
			return table.First(), nil
		}
	}
	return nil, fmt.Errorf("table %s: %w", tableID, ErrTableNotFound)
}

func commentsUnder(sel *goquery.Selection) []string {
	var comments []string
	for _, node := range sel.Nodes {
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			if child.Type == html.CommentNode {
				comments = append(comments, child.Data)
			}
		}
	}
	return comments
}

func readRow(tr *goquery.Selection) (row, bool) {
	player := tr.Find(`[data-stat="player"] a`)
	href, _ := player.Attr("href")
	match := playerIDFromHref.FindStringSubmatch(href)
	if match == nil {
		return row{}, false
	}
	r := row{fbrefID: match[1], cells: map[string]string{}}
	tr.Children().Each(func(_ int, cell *goquery.Selection) {
		if stat, ok := cell.Attr("data-stat"); ok {
			r.cells[stat] = strings.TrimSpace(cell.Text())
		}
	})
	return r, true
}
