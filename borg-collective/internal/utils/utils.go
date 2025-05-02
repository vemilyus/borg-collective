package utils

import "regexp"
import "strings"

var matcher = regexp.MustCompile(`"((?:[^"\\]|\\.)*)"|'((?:[^'\\]|\\.)*)'|(\S+)`)

func SplitCommandLine(input string) []string {
	var result []string
	for _, match := range matcher.FindAllStringSubmatch(input, -1) {
		if match[1] != "" {
			result = append(result, unescape(match[1]))
		} else if match[2] != "" {
			result = append(result, unescape(match[2]))
		} else if match[3] != "" {
			result = append(result, match[3])
		}
	}

	return result
}

func unescape(s string) string {
	s = strings.ReplaceAll(s, "\\\"", "\"")
	s = strings.ReplaceAll(s, "\\'", "'")
	return s
}
