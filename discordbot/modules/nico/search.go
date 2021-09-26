package nico

import (
	"strings"

	"github.com/eientei/jaroid/nicovideo/search"

	"github.com/eientei/jaroid/discordbot/router"
)

func isField(s string) bool {
	switch search.Field(s) {
	case search.FieldContentID,
		search.FieldTitle,
		search.FieldDescription,
		search.FieldUserID,
		search.FieldViewCounter,
		search.FieldMylistCounter,
		search.FieldLengthSeconds,
		search.FieldThumbnailURL,
		search.FieldStartTime,
		search.FieldThreadID,
		search.FieldCommentCounter,
		search.FieldLastCommentTime,
		search.FieldCategoryTags,
		search.FieldChannelID,
		search.FieldTags,
		search.FieldTagsExact,
		search.FieldLockTagsExact,
		search.FieldGenre,
		search.FieldGenreKeyword:
		return true
	}

	return false
}

func isComparableField(s string) bool {
	switch search.Field(s) {
	case search.FieldViewCounter,
		search.FieldMylistCounter,
		search.FieldLengthSeconds,
		search.FieldStartTime,
		search.FieldCommentCounter,
		search.FieldLastCommentTime:
		return true
	}

	return false
}

func parseSearchVariables(a string, s *search.Search) bool {
	parts := strings.Split(a[1:], "=")
	if !isField(parts[0]) {
		s.Query += " " + a
		return true
	}

	if isComparableField(parts[0]) {
		switch {
		case strings.Contains(parts[1], ".."):
			s.Filters = append(s.Filters, search.Filter{
				Field:    search.Field(parts[0]),
				Operator: search.OperatorRange,
				Values:   strings.Split(parts[1], ".."),
			})

			return true
		case strings.HasPrefix(parts[1], ">"):
			s.Filters = append(s.Filters, search.Filter{
				Field:    search.Field(parts[0]),
				Operator: search.OperatorGTE,
				Values:   []string{parts[1][1:]},
			})

			return true
		case strings.HasPrefix(parts[1], "<"):
			s.Filters = append(s.Filters, search.Filter{
				Field:    search.Field(parts[0]),
				Operator: search.OperatorLTE,
				Values:   []string{parts[1][1:]},
			})

			return true
		}
	}

	s.Filters = append(s.Filters, search.Filter{
		Field:    search.Field(parts[0]),
		Operator: search.OperatorEqual,
		Values:   []string{parts[1]},
	})

	return false
}

func (mod *module) parseSearch(args router.Args, fields []search.Field, offset, limit int) (s *search.Search) {
	s = &search.Search{
		Fields: fields,
		Offset: offset,
		Limit:  limit,
	}

	s.SortDirection = search.SortDesc
	s.SortField = search.FieldViewCounter

	for _, a := range args[1:] {
		switch {
		case (strings.HasPrefix(a, "-") || strings.HasPrefix(a, "+")) && isField(a[1:]):
			s.SortDirection, s.SortField = search.SortDirection(a[0]), search.Field(a[1:])
		case strings.HasPrefix(a, "%") && isField(a[1:]):
			s.Targets = append(s.Targets, search.Field(a[1:]))
		case strings.HasPrefix(a, "$") && strings.Contains(a, "="):
			if parseSearchVariables(a, s) {
				continue
			}
		default:
			s.Query += " " + a
		}
	}

	if len(s.Targets) == 0 {
		s.Targets = []search.Field{
			search.FieldTitle,
			search.FieldTags,
			search.FieldDescription,
		}
	}

	return
}
