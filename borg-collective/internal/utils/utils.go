// Copyright (C) 2025 Alex Katlein
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with this program. If not, see <https://www.gnu.org/licenses/>.

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

// ToMap yoink'd from gotest.tools/v3/env

// ToMap takes a list of strings in the format returned by [os.Environ] and
// returns a mapping of keys to values.
func ToMap(env []string) map[string]string {
	result := map[string]string{}
	for _, raw := range env {
		key, value := getParts(raw)
		result[key] = value
	}
	return result
}

func getParts(raw string) (string, string) {
	if raw == "" {
		return "", ""
	}
	// Environment variables on windows can begin with =
	// http://blogs.msdn.com/b/oldnewthing/archive/2010/05/06/10008132.aspx
	parts := strings.SplitN(raw[1:], "=", 2)
	key := raw[:1] + parts[0]
	if len(parts) == 1 {
		return key, ""
	}
	return key, parts[1]
}
