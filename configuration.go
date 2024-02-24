package logging

import (
	"log"
	"strconv"
	"strings"

	"github.com/sharkpick/configuration"
)

var TheLoggerConfig = configuration.New("logging.conf")

const (
	MaxFilesKey      = `MaxFiles`
	CompressFilesKey = `CompressFiles`
)

var (
	DefaultMaxFiles      = 5
	DefaultCompressFiles = true
)

func MaxFiles() int {
	got := TheLoggerConfig.Get(MaxFilesKey)
	n, err := strconv.Atoi(got)
	if err == nil {
		return n
	} else if len(got) > 0 {
		log.Printf("logging.MaxFiles error parsing: %v\n", err)
	}
	return DefaultMaxFiles
}

func CompressFiles() bool {
	got := TheLoggerConfig.Get(CompressFilesKey)
	b, err := strconv.ParseBool(got)
	if err == nil {
		return b
	} else {
		switch strings.ToLower(got) {
		case "on":
			return true
		case "off":
			return false
		case "true":
			return true
		case "false":
			return false
		default:
			return DefaultCompressFiles
		}
	}
}
