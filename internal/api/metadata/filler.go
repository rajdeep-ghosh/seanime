package metadata

import (
	"fmt"
	"github.com/adrg/strutil/metrics"
	"github.com/gocolly/colly"
	"github.com/rs/zerolog"
	"github.com/seanime-app/seanime/internal/util"
	"strings"
)

type (
	FillerSearchOptions struct {
		Titles []string
	}

	FillerSearchResult struct {
		Slug  string
		Title string
	}

	FillerAPI interface {
		Search(opts FillerSearchOptions) (*FillerSearchResult, error)
		FindFillerEpisodes(slug string) ([]string, error)
	}
)

//////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////

type (
	AnimeFillerList struct {
		baseUrl   string
		userAgent string
		logger    *zerolog.Logger
	}
)

func NewAnimeFillerList(logger *zerolog.Logger) *AnimeFillerList {
	return &AnimeFillerList{
		baseUrl:   "https://www.animefillerlist.com",
		userAgent: util.GetRandomUserAgent(),
		logger:    logger,
	}
}

func (af *AnimeFillerList) Search(opts FillerSearchOptions) (result *FillerSearchResult, err error) {

	defer util.HandlePanicInModuleWithError("api/metadata/filler/Search", &err)

	c := colly.NewCollector(
		colly.UserAgent(af.userAgent),
	)

	ret := make([]*FillerSearchResult, 0)

	c.OnHTML("div.Group > ul > li > a", func(e *colly.HTMLElement) {
		ret = append(ret, &FillerSearchResult{
			Slug:  e.Attr("href"),
			Title: e.Text,
		})
	})

	err = c.Visit(fmt.Sprintf("%s/shows", af.baseUrl))
	if err != nil {
		return nil, err
	}

	if len(ret) == 0 {
		return nil, fmt.Errorf("no results found")
	}

	lev := metrics.NewLevenshtein()
	lev.CaseSensitive = false

	compResults := make([]struct {
		OriginalValue string
		Value         string
		Distance      int
	}, 0)

	for _, result := range ret {
		firstTitle := result.Title
		secondTitle := ""

		// Check if a second title exists between parentheses
		if strings.LastIndex(firstTitle, " (") != -1 && strings.LastIndex(firstTitle, ")") != -1 {
			secondTitle = firstTitle[strings.LastIndex(firstTitle, " (")+2 : strings.LastIndex(firstTitle, ")")]
			if !util.IsMostlyLatinString(secondTitle) {
				secondTitle = ""
			}
		}

		if secondTitle != "" {
			firstTitle = firstTitle[:strings.LastIndex(firstTitle, " (")]
		}

		for _, mediaTitle := range opts.Titles {
			compResults = append(compResults, struct {
				OriginalValue string
				Value         string
				Distance      int
			}{
				OriginalValue: result.Title,
				Value:         firstTitle,
				Distance:      lev.Distance(mediaTitle, firstTitle),
			})
			if secondTitle != "" {
				compResults = append(compResults, struct {
					OriginalValue string
					Value         string
					Distance      int
				}{
					OriginalValue: result.Title,
					Value:         secondTitle,
					Distance:      lev.Distance(mediaTitle, secondTitle),
				})
			}
		}
	}

	// Find the best match
	bestResult := struct {
		OriginalValue string
		Value         string
		Distance      int
	}{}

	for _, result := range compResults {
		if bestResult.OriginalValue == "" || result.Distance <= bestResult.Distance {
			if bestResult.OriginalValue != "" && result.Distance == bestResult.Distance && len(result.OriginalValue) > len(bestResult.OriginalValue) {
				continue
			}
			bestResult = result
		}
	}

	if bestResult.OriginalValue == "" {
		return nil, fmt.Errorf("no results found")
	}

	// Get the result
	for _, r := range ret {
		if r.Title == bestResult.OriginalValue {
			return r, nil
		}
	}

	return
}

func (af *AnimeFillerList) FindFillerEpisodes(slug string) (ret []string, err error) {

	defer util.HandlePanicInModuleWithError("api/metadata/filler/FindFillerEpisodes", &err)

	c := colly.NewCollector(
		colly.UserAgent(af.userAgent),
	)

	ret = make([]string, 0)

	c.OnHTML("tr.filler", func(e *colly.HTMLElement) {
		ret = append(ret, e.ChildText("td.Number"))
	})

	err = c.Visit(fmt.Sprintf("%s%s", af.baseUrl, slug))
	if err != nil {
		return nil, err
	}

	return
}
