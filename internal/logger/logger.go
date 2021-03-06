package log

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"runtime"
	"sync"
	"time"

	"golang.org/x/net/context"

	"cloud.google.com/go/logging"
)

// These flags define which text to prefix to each log entry generated by the Logger.
// Bits are or'ed together to control what's printed.
// There is no control over the order they appear (the order listed
// here) or the format they present (as described in the comments).
// The prefix is followed by a colon only when Llongfile or Lshortfile
// is specified.
// For example, flags Ldate | Ltime (or LstdFlags) produce,
//	2009/01/23 01:23:23 message
// while flags Ldate | Ltime | Lmicroseconds | Llongfile produce,
//	2009/01/23 01:23:23.123123 /a/b/c/d.go:23: message
const (
	Ldate         = 1 << iota     // the date in the local time zone: 2009/01/23
	Ltime                         // the time in the local time zone: 01:23:23
	Lmicroseconds                 // microsecond resolution: 01:23:23.123123.  assumes Ltime.
	Llongfile                     // full file name and line number: /a/b/c/d.go:23
	Lshortfile                    // final file name element and line number: d.go:23. overrides Llongfile
	LUTC                          // if Ldate or Ltime is set, use UTC rather than the local time zone
	LstdFlags     = Ldate | Ltime // initial values for the standard logger
)

type Logger struct {
	mu                *sync.Mutex // ensures atomic writes; protects the following fields
	prefix            string      // prefix to write at beginning of each line
	debug             bool
	flag              int // properties
	stackDriverLogger *logging.Logger
	loggingClient     *logging.Client
	httpRequest       *logging.HTTPRequest
	buf               []byte // for accumulating text to write
	defaultSeverity   logging.Severity
	local             bool
}
type LoggerOption func(*LoggerOptions)
type LoggerOptions struct {
	LogName         string
	Prefix          string
	Debug           bool
	Local           bool
	DefaultSeverity logging.Severity
	Context         context.Context
}

func WithLogName(logname string) LoggerOption {
	return func(o *LoggerOptions) {
		o.LogName = logname
	}
}
func WithLocal(local bool) LoggerOption {
	return func(o *LoggerOptions) {
		o.Local = local
	}
}
func WithPrefix(prefix string) LoggerOption {
	return func(o *LoggerOptions) {
		o.Prefix = prefix
	}
}
func WithDefaultSeverity(defaultSeverity logging.Severity) LoggerOption {
	return func(o *LoggerOptions) {
		o.DefaultSeverity = defaultSeverity
	}
}
func WithDebug(debug bool) LoggerOption {
	return func(o *LoggerOptions) {
		o.Debug = debug
	}
}
func WithContext(ctx context.Context) LoggerOption {
	return func(o *LoggerOptions) {
		o.Context = ctx
	}
}

func New(projectID string, options ...LoggerOption) *Logger {
	opts := LoggerOptions{
		LogName:         "",
		Prefix:          os.Args[0] + ": ",
		DefaultSeverity: logging.Error,
		Debug:           false,
		Local:           false,
		Context:         context.Background(),
	}
	for _, o := range options {
		o(&opts)
	}

	logger := &Logger{mu: new(sync.Mutex), prefix: opts.Prefix, debug: opts.Debug, local: opts.Local, flag: Lshortfile | LstdFlags}
	loggingClient, err := logging.NewClient(opts.Context, projectID)
	if err != nil {
		log.Fatalf("Failed to create logging client: %v", err)
	}
	logger.loggingClient = loggingClient
	logger.stackDriverLogger = loggingClient.Logger(opts.LogName)
	return logger
}

var std = newLocal()

func newLocal() *Logger {
	return &Logger{mu: new(sync.Mutex), prefix: "", local: true}
}

// WithRequest returns a shallow copy of logger with a request present
func (l *Logger) WithRequest(r *http.Request) *Logger {
	l.mu.Lock()
	defer l.mu.Unlock()
	if r == nil || l == nil {
		panic("nil request")
	}
	l2 := new(Logger)
	*l2 = *l
	l2.httpRequest = &logging.HTTPRequest{Request: r}
	return l2
}

func (l *Logger) Info(message interface{}) {
	l.outputEntry(2, logging.Entry{
		Payload:  message,
		Severity: logging.Info,
	})
}
func (l *Logger) Debug(message interface{}) {
	if l.debug {
		l.outputEntry(2, logging.Entry{
			Payload:  message,
			Severity: logging.Debug,
		})
	}
}
func (l *Logger) Error(message interface{}) {
	l.outputEntry(2, logging.Entry{
		Payload:  message,
		Severity: logging.Error,
	})
}
func (l *Logger) Critical(message interface{}) {
	l.outputEntry(2, logging.Entry{
		Payload:  message,
		Severity: logging.Critical,
	})
}

func (l *Logger) Infof(format string, a ...interface{}) {
	l.outputEntry(2, logging.Entry{
		Payload:  fmt.Sprintf(format, a...),
		Severity: logging.Info,
	})
}
func (l *Logger) Debugf(format string, a ...interface{}) {
	if l.debug {
		l.outputEntry(2, logging.Entry{
			Payload:  fmt.Sprintf(format, a...),
			Severity: logging.Debug,
		})
	}
}
func (l *Logger) Errorf(format string, a ...interface{}) {
	l.outputEntry(2, logging.Entry{
		Payload:  fmt.Sprintf(format, a...),
		Severity: logging.Error,
	})
}
func (l *Logger) Criticalf(format string, a ...interface{}) {
	l.outputEntry(2, logging.Entry{
		Payload:  fmt.Sprintf(format, a...),
		Severity: logging.Critical,
	})
}

func (l *Logger) Close() {
	l.loggingClient.Close()
}

// Output writes the output for a logging event. The string s contains
// the text to print after the prefix specified by the flags of the
// Logger. A newline is appended if the last character of s is not
// already a newline. Calldepth is used to recover the PC and is
// provided for generality, although at the moment on all pre-defined
// paths it will be 2.
func (l *Logger) Output(calldepth int, s string) error {
	e := logging.Entry{
		Severity: l.defaultSeverity,
		Payload:  s,
	}
	if l.httpRequest != nil {
		e.HTTPRequest = l.httpRequest
	}
	return l.outputEntry(calldepth+1, e)

}

func (l *Logger) outputEntry(calldepth int, entry logging.Entry) error {
	now := time.Now() // get this early.
	var file string
	var line int
	l.mu.Lock()
	defer l.mu.Unlock()

	s, ok := entry.Payload.(string)
	if !ok {
		return errors.New("failed to convert payload to string")
	}

	if l.flag&(Lshortfile|Llongfile) != 0 {
		// Release lock while getting caller info - it's expensive.
		l.mu.Unlock()
		var ok bool
		_, file, line, ok = runtime.Caller(calldepth)
		if !ok {
			file = "???"
			line = 0
		}
		l.mu.Lock()
	}
	l.buf = l.buf[:0]
	l.formatHeader(&l.buf, now, file, line)
	l.buf = append(l.buf, s...)
	if len(s) == 0 || s[len(s)-1] != '\n' {
		l.buf = append(l.buf, '\n')
	}
	if l.local {
		os.Stdout.Write(l.buf)
	} else {
		entry.Payload = string(l.buf[:])
		if l.httpRequest != nil && entry.HTTPRequest == nil {
			entry.HTTPRequest = l.httpRequest
		}
		l.stackDriverLogger.Log(entry)
	}
	return nil
}

// formatHeader writes log header to buf in following order:
//   * l.prefix (if it's not blank),
//   * date and/or time (if corresponding flags are provided),
//   * file and line number (if corresponding flags are provided).
func (l *Logger) formatHeader(buf *[]byte, t time.Time, file string, line int) {
	*buf = append(*buf, l.prefix...)
	if l.flag&(Ldate|Ltime|Lmicroseconds) != 0 {
		if l.flag&LUTC != 0 {
			t = t.UTC()
		}
		if l.flag&Ldate != 0 {
			year, month, day := t.Date()
			itoa(buf, year, 4)
			*buf = append(*buf, '/')
			itoa(buf, int(month), 2)
			*buf = append(*buf, '/')
			itoa(buf, day, 2)
			*buf = append(*buf, ' ')
		}
		if l.flag&(Ltime|Lmicroseconds) != 0 {
			hour, min, sec := t.Clock()
			itoa(buf, hour, 2)
			*buf = append(*buf, ':')
			itoa(buf, min, 2)
			*buf = append(*buf, ':')
			itoa(buf, sec, 2)
			if l.flag&Lmicroseconds != 0 {
				*buf = append(*buf, '.')
				itoa(buf, t.Nanosecond()/1e3, 6)
			}
			*buf = append(*buf, ' ')
		}
	}
	if l.flag&(Lshortfile|Llongfile) != 0 {
		if l.flag&Lshortfile != 0 {
			short := file
			for i := len(file) - 1; i > 0; i-- {
				if file[i] == '/' {
					short = file[i+1:]
					break
				}
			}
			file = short
		}
		*buf = append(*buf, file...)
		*buf = append(*buf, ':')
		itoa(buf, line, -1)
		*buf = append(*buf, ": "...)
	}
}

// Cheap integer to fixed-width decimal ASCII. Give a negative width to avoid zero-padding.
func itoa(buf *[]byte, i int, wid int) {
	// Assemble decimal in reverse order.
	var b [20]byte
	bp := len(b) - 1
	for i >= 10 || wid > 1 {
		wid--
		q := i / 10
		b[bp] = byte('0' + i - q*10)
		bp--
		i = q
	}
	// i < 10
	b[bp] = byte('0' + i)
	*buf = append(*buf, b[bp:]...)
}

// Flags returns the output flags for the logger.
func (l *Logger) Flags() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.flag
}

// SetFlags sets the output flags for the logger.
func (l *Logger) SetFlags(flag int) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.flag = flag
}

// Prefix returns the output prefix for the logger.
func (l *Logger) Prefix() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.prefix
}

// SetPrefix sets the output prefix for the logger.
func (l *Logger) SetPrefix(prefix string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.prefix = prefix
}

// These functions write to the standard logger.

// Print calls Output to print to the standard logger.
// Arguments are handled in the manner of fmt.Print.
func Print(v ...interface{}) {
	std.Output(2, fmt.Sprint(v...))
}

// Printf calls Output to print to the standard logger.
// Arguments are handled in the manner of fmt.Printf.
func Printf(format string, v ...interface{}) {
	std.Output(2, fmt.Sprintf(format, v...))
}

// Println calls Output to print to the standard logger.
// Arguments are handled in the manner of fmt.Println.
func Println(v ...interface{}) {
	std.Output(2, fmt.Sprintln(v...))
}

// Fatal is equivalent to Print() followed by a call to os.Exit(1).
func Fatal(v ...interface{}) {
	std.Output(2, fmt.Sprint(v...))
	os.Exit(1)
}

// Fatalf is equivalent to Printf() followed by a call to os.Exit(1).
func Fatalf(format string, v ...interface{}) {
	std.Output(2, fmt.Sprintf(format, v...))
	os.Exit(1)
}

// Fatalln is equivalent to Println() followed by a call to os.Exit(1).
func Fatalln(v ...interface{}) {
	std.Output(2, fmt.Sprintln(v...))
	os.Exit(1)
}

// Panic is equivalent to Print() followed by a call to panic().
func Panic(v ...interface{}) {
	s := fmt.Sprint(v...)
	std.Output(2, s)
	panic(s)
}

// Panicf is equivalent to Printf() followed by a call to panic().
func Panicf(format string, v ...interface{}) {
	s := fmt.Sprintf(format, v...)
	std.Output(2, s)
	panic(s)
}

// Panicln is equivalent to Println() followed by a call to panic().
func Panicln(v ...interface{}) {
	s := fmt.Sprintln(v...)
	std.Output(2, s)
	panic(s)
}

// Output writes the output for a logging event. The string s contains
// the text to print after the prefix specified by the flags of the
// Logger. A newline is appended if the last character of s is not
// already a newline. Calldepth is the count of the number of
// frames to skip when computing the file name and line number
// if Llongfile or Lshortfile is set; a value of 1 will print the details
// for the caller of Output.
func Output(calldepth int, s string) error {
	return std.Output(calldepth+1, s) // +1 for this frame.
}
