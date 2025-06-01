package utils

import (
	"fmt"
	"regexp"
	"time"
)

var normalizationRegexp = regexp.MustCompile("[^_a-zA-Z0-9]+")

func ArchiveName(baseName string) string {
	normalizedName := normalizationRegexp.ReplaceAllString(baseName, "_")
	return fmt.Sprintf("%s-%s", normalizedName, time.Now().Format("20060102150405"))
}
