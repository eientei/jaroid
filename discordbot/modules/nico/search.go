package nico

import (
	"strings"

	"github.com/eientei/jaroid/discordbot/router"
	"github.com/eientei/jaroid/integration/nicovideo"
)

func isField(s string) bool {
	switch nicovideo.Field(s) {
	case nicovideo.FieldContentID,
		nicovideo.FieldTitle,
		nicovideo.FieldDescription,
		nicovideo.FieldUserID,
		nicovideo.FieldViewCounter,
		nicovideo.FieldMylistCounter,
		nicovideo.FieldLengthSeconds,
		nicovideo.FieldThumbnailURL,
		nicovideo.FieldStartTime,
		nicovideo.FieldThreadID,
		nicovideo.FieldCommentCounter,
		nicovideo.FieldLastCommentTime,
		nicovideo.FieldCategoryTags,
		nicovideo.FieldChannelID,
		nicovideo.FieldTags,
		nicovideo.FieldTagsExact,
		nicovideo.FieldLockTagsExact,
		nicovideo.FieldGenre,
		nicovideo.FieldGenreKeyword:
		return true
	}

	return false
}

func isComparableField(s string) bool {
	switch nicovideo.Field(s) {
	case nicovideo.FieldViewCounter,
		nicovideo.FieldMylistCounter,
		nicovideo.FieldLengthSeconds,
		nicovideo.FieldStartTime,
		nicovideo.FieldCommentCounter,
		nicovideo.FieldLastCommentTime:
		return true
	}

	return false
}

func parseSearchVariables(a string, s *nicovideo.Search) bool {
	parts := strings.Split(a[1:], "=")
	if !isField(parts[0]) {
		s.Query += " " + a
		return true
	}

	if isComparableField(parts[0]) {
		switch {
		case strings.Contains(parts[1], ".."):
			s.Filters = append(s.Filters, nicovideo.Filter{
				Field:    nicovideo.Field(parts[0]),
				Operator: nicovideo.OperatorRange,
				Values:   strings.Split(parts[1], ".."),
			})

			return true
		case strings.HasPrefix(parts[1], ">"):
			s.Filters = append(s.Filters, nicovideo.Filter{
				Field:    nicovideo.Field(parts[0]),
				Operator: nicovideo.OperatorGTE,
				Values:   []string{parts[1][1:]},
			})

			return true
		case strings.HasPrefix(parts[1], "<"):
			s.Filters = append(s.Filters, nicovideo.Filter{
				Field:    nicovideo.Field(parts[0]),
				Operator: nicovideo.OperatorLTE,
				Values:   []string{parts[1][1:]},
			})

			return true
		}
	}

	s.Filters = append(s.Filters, nicovideo.Filter{
		Field:    nicovideo.Field(parts[0]),
		Operator: nicovideo.OperatorEqual,
		Values:   []string{parts[1]},
	})

	return false
}

func (mod *module) parseSearch(args router.Args, fields []nicovideo.Field, offset, limit int) (s *nicovideo.Search) {
	s = &nicovideo.Search{
		Fields: fields,
		Offset: offset,
		Limit:  limit,
	}

	s.SortDirection = nicovideo.SortDesc
	s.SortField = nicovideo.FieldViewCounter

	for _, a := range args[1:] {
		switch {
		case (strings.HasPrefix(a, "-") || strings.HasPrefix(a, "+")) && isField(a[1:]):
			s.SortDirection, s.SortField = nicovideo.SortDirection(a[0]), nicovideo.Field(a[1:])
		case strings.HasPrefix(a, "%") && isField(a[1:]):
			s.Targets = append(s.Targets, nicovideo.Field(a[1:]))
		case strings.HasPrefix(a, "$") && strings.Contains(a, "="):
			if parseSearchVariables(a, s) {
				continue
			}
		default:
			s.Query += " " + a
		}
	}

	if len(s.Targets) == 0 {
		s.Targets = []nicovideo.Field{
			nicovideo.FieldTitle,
			nicovideo.FieldTags,
			nicovideo.FieldDescription,
		}
	}

	return
}
