# package logging

logging provides a very simple system for producing/managing logfiles from your Go program. It is used in places where you can't use syslog. In general, syslog provides a much better interface for logging and should be used in place of this package wherever possible. Parameters such as how many logfiles to keep and whether to compress the old files are configurable during program run through the [configuration package](https://github.com/sharkpick/configuration)

## use
```go
package main

import (
    "github.com/sharkpick/logging"
)

const (
    LogFilename = "/var/log/TheTestProgram/TheProgramLog.log"
)

func main() {
    if logger, err := logging.New(LogFilename); err != nil {
        panic("error opening logger: "+err.Error())
    } else {
        defer logger.Close()
        log.SetOutput(logger)
    }
    // proceed as usual
}
```