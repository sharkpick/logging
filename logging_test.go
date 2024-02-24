package logging

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"log"
	"math/rand"
	"os"
	"strings"
	"testing"
	"time"
)

var TheTestWords = func() []string {
	f, err := os.Open("/usr/share/dict/words")
	if err != nil {
		panic("error opening words: " + err.Error())
	}
	defer f.Close()
	results := make([]string, 0)
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		if line := strings.TrimSpace(scanner.Text()); len(line) > 0 {
			results = append(results, line)
		}
	}
	return results
}()

func RandomWord() string { return TheTestWords[rand.Intn(len(TheTestWords))] }

const (
	TheTestFilename  = "TestFilename.log"
	TheTestFilepath  = "TestFilename"
	TheTestExtension = ".log"
)

var (
	TheDefaultConfig = LoggerFilename{TheTestFilename}
)

func TestLogger(t *testing.T) {
	logger, err := New(TheTestFilename)
	if err != nil {
		t.Fatalf("error opening logger: %v\n", err)
	}
	defer func() {
		if err := logger.Close(); err != nil {
			t.Fatalf("error closing: %v\n", err)
		} else if err := os.Remove(TheDefaultConfig.TodaysLogfile()); err != nil {
			t.Fatalf("error removing %s: %v\n", TheDefaultConfig.Filename, err)
		}
	}()
	// write to logs
	want := new(bytes.Buffer)
	for i := 0; i < 10; i++ {
		word := RandomWord() + "\n"
		want.WriteString(word)
		logger.Write([]byte(word))
	}
	// now flush and compare the file
	logger.buffer.Flush()
	got, err := os.ReadFile(logger.f.Name())
	if err != nil {
		t.Fatalf("error opening %s: %v\n", logger.f.Name(), err)
	}
	if want, got := want.Bytes(), got; !bytes.Equal(want, got) {
		t.Fatalf("error: wanted %s; got %s\n", want, got)
	}
}

func TestLogRotation(t *testing.T) {
	DefaultCompressFiles = false
	TimeNow = func() time.Time { return time.Now().Add(-time.Hour * 24) } // pretend its yesterday
	logger, err := New(TheTestFilename)
	if err != nil {
		t.Fatalf("error opening logger: %v\n", err)
	}
	defer func() {
		if err := logger.Close(); err != nil {
			t.Fatalf("error closing logger: %v\n", err)
		} else if err := os.Remove(TheDefaultConfig.YesterdaysLogfile()); err != nil {
			t.Fatalf("error removing %s: %v\n", TheDefaultConfig.YesterdaysLogfile(), err)
		} else if err := os.Remove(TheDefaultConfig.TodaysLogfile()); err != nil {
			t.Fatalf("error removing %s: %v\n", TheDefaultConfig.TodaysLogfile(), err)
		}
	}()
	buffer1 := new(bytes.Buffer)
	for i := 0; i < 10; i++ {
		word := fmt.Sprintf("%s\n", RandomWord())
		buffer1.WriteString(word)
		logger.Write([]byte(word))
	}
	// it's today again - next write will force log rotation
	TimeNow = func() time.Time { return time.Now() }
	buffer2 := new(bytes.Buffer)
	for i := 0; i < 10; i++ {
		word := fmt.Sprintf("%s\n", RandomWord())
		buffer2.WriteString(word)
		logger.Write([]byte(word))
	}
	// flush the current logfile
	logger.buffer.Flush()
	got1, err := os.ReadFile(TheDefaultConfig.YesterdaysLogfile())
	if err != nil {
		t.Fatalf("error reading first logfile: %v\n", err)
	}
	got2, err := os.ReadFile(TheDefaultConfig.TodaysLogfile())
	if err != nil {
		t.Fatalf("error reading second logfile: %v\n", err)
	}
	if want, got := buffer1.Bytes(), got1; !bytes.Equal(want, got) {
		t.Fatalf("error: wanted %s; got %s\n", want, got)
	} else if want, got := buffer2.Bytes(), got2; !bytes.Equal(want, got) {
		t.Fatalf("error: wanted %s; got %s\n", want, got)
	}
	DefaultCompressFiles = true
}

func TestFindLogFiles(t *testing.T) {
	want := make(logfiles, 0, 10)
	for i := 10; i > 0; i-- {
		timestampstring := time.Now().Add(-time.Hour * 24 * time.Duration(i)).Format(TimestampFormat)
		timestamp, _ := time.Parse(TimestampFormat, timestampstring)
		filename := fmt.Sprintf("%s-%s%s", TheTestFilepath, timestampstring, TheTestExtension)
		func() {
			f, err := os.Create(filename)
			if err != nil {
				t.Fatalf("error creating %s: %v\n", filename, err)
			}
			defer f.Close()
		}()
		want = append(want, logfile{filename: filename, timestamp: timestamp})
	}
	defer func() {
		for i := range want {
			if err := os.Remove(want[i].filename); err != nil {
				log.Printf("error removing %s: %v\n", want[i].filename, err)
			}
		}
	}()
	got, err := FindLogFiles(TheDefaultConfig)
	if err != nil {
		t.Fatalf("error finding log files: %v\n", err)
	}
	if len(want) != len(got) {
		t.Fatalf("error: wanted %d; got %d\n", len(want), len(got))
	}
	for i := range want {
		if want, got := want[i], got[i]; want != got {
			t.Fatalf("error: wanted %v; got %v\n", want, got)
		}
	}

}

func TestCleanupOldLogfiles(t *testing.T) {
	want := make(logfiles, 0, 10)
	for i := 10; i > 0; i-- {
		timestampstring := time.Now().Add(-time.Hour * 24 * time.Duration(i)).Format(TimestampFormat)
		timestamp, _ := time.Parse(TimestampFormat, timestampstring)
		filename := fmt.Sprintf("%s-%s%s", TheTestFilepath, timestampstring, TheTestExtension)
		func() {
			f, err := os.Create(filename)
			if err != nil {
				t.Fatalf("error creating %s: %v\n", filename, err)
			}
			defer f.Close()
		}()
		want = append(want, logfile{filename: filename, timestamp: timestamp})
	}
	logger, err := New(TheTestFilename)
	if err != nil {
		for _, file := range want {
			os.Remove(file.filename)
		}
		t.Fatalf("error opening logger: %v\n", err)
	}
	defer func() {
		filename := logger.f.Name()
		if err := logger.Close(); err != nil {
			t.Fatalf("error closing logger: %v\n", err)
		} else if err := os.Remove(filename); err != nil {
			t.Fatalf("error deleting active logfile %s: %v\n", filename, err)
		}
		for _, file := range want {
			if err := os.Remove(file.filename); err == nil {
				t.Fatalf("error: found/deleted old filename that should have been cleaned up...")
			}
		}
	}()
	DefaultMaxFiles = 0
	if err := CleanupOldLogfiles(TheDefaultConfig); err != nil {
		t.Fatalf("error cleaning up logfiles: %v\n", err)
	}
	DefaultMaxFiles = 5
}

func TestLoggerWithLogPackage(t *testing.T) {
	logger, err := New(TheTestFilename)
	if err != nil {
		t.Fatalf("error opening logger: %v\n", err)
	}
	defer func() {
		filename := logger.f.Name()
		if err := logger.Close(); err != nil {
			t.Fatalf("error closing logger: %v\n", err)
		} else if err := os.Remove(filename); err != nil {
			t.Fatalf("error removing active logfile %s: %v\n", filename, err)
		}
	}()
	log.SetOutput(logger)
	remember_flags := log.Flags()
	log.SetFlags(0)
	want := new(bytes.Buffer)
	for i := 0; i < 10; i++ {
		word := fmt.Sprintf("%s\n", RandomWord())
		want.WriteString(word)
		log.Print(word)
	}
	if err := logger.buffer.Flush(); err != nil {
		t.Fatalf("error flushing buffer: %v\n", err)
	} else if got, err := os.ReadFile(logger.f.Name()); err != nil {
		t.Fatalf("error reading %s: %v\n", logger.f.Name(), err)
	} else if !bytes.Equal(want.Bytes(), got) {
		t.Fatalf("error: wanted %s; got %s\n", want.Bytes(), got)
	}
	log.SetOutput(os.Stdout)
	log.SetFlags(remember_flags)
}

func TestCompression(t *testing.T) {
	filename := TheDefaultConfig.TodaysLogfile()
	want := func() []byte {
		buffer := new(bytes.Buffer)
		f, err := os.Create(filename)
		if err != nil {
			t.Fatalf("error creating %s: %v\n", filename, err)
		}
		defer f.Close()
		for i := 0; i < 10; i++ {
			word := fmt.Sprintf("%s\n", RandomWord())
			f.WriteString(word)
			buffer.WriteString(word)
		}
		return buffer.Bytes()
	}()
	if err := CompressLogfile(filename); err != nil {
		t.Fatalf("error compressing %s: %v\n", filename, err)
	} else if _, err := os.Stat(filename); err != nil && !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("error: wanted ErrNotExist for original file after successful compresson; got %v\n", err)
	}
	compressed_filename := filename + CompressedLogfileSuffix
	got := func() []byte {
		f, err := os.Open(compressed_filename)
		if err != nil {
			t.Fatalf("error opening %s: %v\n", compressed_filename, err)
		}
		defer f.Close()
		reader, err := gzip.NewReader(f)
		if err != nil {
			t.Fatalf("error opening reader: %v\n", err)
		}
		defer reader.Close()
		buffer := new(bytes.Buffer)
		if _, err := io.Copy(buffer, reader); err != nil {
			t.Fatalf("error copying: %v\n", err)
		}
		return buffer.Bytes()
	}()
	defer func() {
		if err := os.Remove(compressed_filename); err != nil {
			t.Fatalf("error removing %s: %v\n", compressed_filename, err)
		}
	}()
	if !bytes.Equal(want, got) {
		t.Fatalf("error: wanted %s; got %s\n", want, got)
	}

}
