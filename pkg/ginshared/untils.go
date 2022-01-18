package ginshared

import "strings"

func DropDuplicated(raw []string) []string {
	filterd := make([]string, 0)
	set := make(map[string]bool)
	for _, item := range raw {
		if set[item] {
			continue
		}
		item = strings.TrimSpace(item)
		set[item] = true
		filterd = append(filterd, item)
	}
	return filterd
}
