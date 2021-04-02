package nico

import "fmt"

func formatLength(t int) string {
	return fmt.Sprintf("%d:%02d", t/60, t%60)
}

func formatTags(tags []string) (s string) {
	for i, t := range tags {
		if i > 0 {
			s += " "
		}

		s += "`" + t + "`"
	}

	return
}

func formatNumber(i int) string {
	switch i {
	case 0:
		return emojiOne
	case 1:
		return emojiTwo
	case 2:
		return emojiThree
	case 3:
		return emojiFour
	case 4:
		return emojiFive
	}

	return ""
}

func parseNumber(emoji string) int {
	switch emoji {
	case emojiOne:
		return 0
	case emojiTwo:
		return 1
	case emojiThree:
		return 2
	case emojiFour:
		return 3
	case emojiFive:
		return 4
	}

	return 0
}
