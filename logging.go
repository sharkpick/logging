package logging

import (
	"bufio"
	"compress/gzip"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

var TimeNow = func() time.Time { return time.Now() }

const (
	TimestampFormat         = "20060102"
	CompressedLogfileSuffix = ".gz"
)

type LoggerFilename struct {
	Filename string
}

func (l LoggerFilename) TodaysLogfile() string {
	extension := filepath.Ext(l.Filename)
	path := strings.TrimSuffix(l.Filename, extension)
	timestamp := TimeNow().Format(TimestampFormat)
	return fmt.Sprintf("%s-%s%s", path, timestamp, extension)
}

func (l LoggerFilename) YesterdaysLogfile() string {
	extension := filepath.Ext(l.Filename)
	path := strings.TrimSuffix(l.Filename, extension)
	timestamp := TimeNow().Add(-time.Hour * 24).Format(TimestampFormat)
	return fmt.Sprintf("%s-%s%s", path, timestamp, extension)
}

type Logger struct {
	f      *os.File
	buffer *bufio.Writer
	mutex  sync.Mutex
	config LoggerFilename
}

func NewFromConfig(config LoggerFilename) (*Logger, error) {
	f, err := os.OpenFile(config.TodaysLogfile(), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0664)
	if err != nil {
		return nil, err
	}
	buffer := bufio.NewWriter(f)
	return &Logger{
		config: config,
		f:      f,
		buffer: buffer,
	}, nil
}

func New(filename string) (*Logger, error) {
	return NewFromConfig(LoggerFilename{Filename: filename})
}

func (l *Logger) Close() error {
	l.mutex.Lock()
	defer l.mutex.Unlock()
	return l.lockedClose()
}

func (l *Logger) lockedClose() error {
	filename := l.f.Name()
	if err := l.buffer.Flush(); err != nil {
		return fmt.Errorf("error flushing *bufio.Writer: %w", err)
	} else if err := l.f.Close(); err != nil {
		return fmt.Errorf("error closing logfile %s: %w", filename, err)
	} else {
		return nil
	}
}

func CompressLogfile(filename string) (err error) {
	defer func() {
		if err == nil {
			err = os.Remove(filename)
		}
	}()
	var infile, outfile *os.File
	infile, err = os.Open(filename)
	if err != nil {
		return
	}
	defer infile.Close()
	outfile, err = os.Create(filename + CompressedLogfileSuffix)
	if err != nil {
		return
	}
	defer outfile.Close()
	writer := gzip.NewWriter(outfile)
	defer writer.Close()
	_, err = io.Copy(writer, infile)
	return
}

type logfile struct {
	filename  string
	timestamp time.Time
}

type logfiles []logfile

func FindLogFiles(config LoggerFilename) (logfiles, error) {
	extension := filepath.Ext(config.Filename)
	path := strings.TrimSuffix(config.Filename, extension)
	f, err := filepath.Glob(path + "*")
	if err != nil {
		return nil, err
	}
	results := make(logfiles, 0, len(f))
	for _, filename := range f {
		buffer := filename
		var found bool
		if buffer, found = strings.CutPrefix(buffer, path+"-"); !found {
			log.Printf("FindLogFiles error cutting prefix from %s\n", filename)
			continue
		}
		buffer = strings.TrimSuffix(strings.TrimSuffix(buffer, CompressedLogfileSuffix), extension)
		timestamp, err := time.Parse(TimestampFormat, buffer)
		if err != nil {
			log.Printf("FindLogFiles error parsing %s: %v\n", filename, err)
			continue
		}
		results = append(results, logfile{filename: filename, timestamp: timestamp})
	}
	return results, nil
}

func CleanupOldLogfiles(config LoggerFilename) error {
	files, err := FindLogFiles(config)
	if err != nil {
		return err
	}
	sort.Slice(files, func(i, j int) bool { return files[j].timestamp.Before(files[i].timestamp) })
	todaysfile := config.TodaysLogfile() // do not delete current logfile
	if want, got := MaxFiles(), len(files); got > want {
		errs := make([]any, 0)
		remove := files[want:]
		for _, file := range remove {
			if file.filename == todaysfile {
				continue
			} else if err := os.Remove(file.filename); err != nil {
				errs = append(errs, err)
			}
		}
		if len(errs) > 0 {
			return fmt.Errorf(strings.Repeat("%w ", len(errs)), errs...)
		}
	}
	return nil
}

func cleanup(config LoggerFilename) {
	// complete rotation and cleanup
	if err := CleanupOldLogfiles(config); err != nil {
		log.Printf("Logger::Write error cleaning up old log files: %v\n", err)
	}
}

func (l *Logger) rotate() error {
	if err := l.lockedClose(); err != nil {
		return fmt.Errorf("error flushing/closing old file: %w", err)
	} else if f, err := os.OpenFile(l.config.TodaysLogfile(), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0664); err != nil {
		return fmt.Errorf("error opening %s: %w", l.config.TodaysLogfile(), err)
	} else {
		l.f = f
		l.buffer = bufio.NewWriter(l.f)
		return nil
	}
}

func (l *Logger) Write(b []byte) (wrote int, err error) {
	func() {
		l.mutex.Lock()
		defer l.mutex.Unlock()
		if want, got := l.f.Name(), l.config.TodaysLogfile(); want != got {
			if err = l.rotate(); err != nil {
				log.Panicf("Logger::Write error rotating logfile: %v\n", err)
				go func() {
					if CompressFiles() {
						logfile := l.config.YesterdaysLogfile()
						if err := CompressLogfile(logfile); err != nil {
							log.Printf("Logger::Write error compressing %s: %v\n", logfile, err)
						}
					}
					cleanup(l.config)
				}()
			}
			l.buffer = bufio.NewWriter(l.f)
		}
		wrote, err = l.buffer.Write(b)
	}()
	return
}
