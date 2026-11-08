package main

import (
	"os"

	"github.com/itsvlxd/stacklog"
)

func main() {
	// 1. Default config, every level.
	base := stacklog.CreateLogger(stacklog.Config{})

	base.Debug("default config, debug level")
	base.Info("default config, info level")
	base.Warn("default config, warn level")
	base.Error("default config, error level")

	// 2. A full style set passed through Config. A component left as the zero
	//    value falls back to the package default for that component.
	//    The arrays are sized by MaxLevel, which is exported.
	custom := stacklog.CreateLogger(stacklog.Config{
		Timestamp:  true,
		TimeFormat: "15:04:05",
		Style: stacklog.Style{
			Timestamp: stacklog.NewStyle().Foreground("#666666"),

			Level: [stacklog.MaxLevel]stacklog.CompStyle{
				stacklog.NewStyle().Foreground("#888888"),                                              // debug
				stacklog.NewStyle().Foreground("#5F87D7"),                                              // info
				stacklog.NewStyle().Foreground("#D7AF5F"),                                              // warn
				stacklog.NewStyle().Foreground("#D75F5F").Padding(stacklog.Padding{Left: 1, Right: 1}), // error
				stacklog.NewStyle().Foreground("#FFFFFF").Background("#D75F5F"),                        // fatal
			},

			Params: [stacklog.MaxLevel]stacklog.ParamsStyle{
				{},
				{
					Param: stacklog.NewStyle().Foreground("#5F87D7"),
					Equal: stacklog.NewStyle().Foreground("#666666"),
					Value: stacklog.NewStyle().Foreground("#AFAFAF"),
				},
				{},
				{
					Param: stacklog.NewStyle().Foreground("#D75F5F"),
					Equal: stacklog.NewStyle().Foreground("#666666"),
					Value: stacklog.NewStyle().Foreground("#FFFFFF"),
				},
				{},
			},
		},
	})

	custom.Debug("every level has its own colour")
	custom.Info("info with a blue key")
	custom.Warn("warn level, default params style")
	custom.Error("padded error badge")
	custom.WithParams("request_id", 42, "user", "vlad").Error("params through WithParams, not as extra arguments")

	// 3. Prefix style and the copy pattern. WithParams and WithPrefix return new
	//    loggers, so a base logger can be shared and specialised per call site.
	scoped := custom.WithPrefix("http")
	paired := scoped.WithParams("request_id", 42, "user", "vlad")

	paired.Warn("params on the shared logger")
	scoped.Info("this logger has no params, the original is unchanged")

	// 4. Padding. Left and Right sit inside the coloured block, so a badge with a
	//    background extends over them. Top applies only at the start of a line,
	//    Bottom only after the line ends.
	badge := stacklog.CreateLogger(stacklog.Config{})

	badge.Config.Style.Level[3] = stacklog.NewStyle().
		Padding(stacklog.Padding{Top: 1, Left: 1, Right: 1, Bottom: 2}).
		Background("#D75F5F").
		Foreground("#FFFFFF")

	badge.Error("Top puts a blank line before, Bottom puts two after")
	badge.Info("next line, no padding")

	// 5. Writing somewhere other than stdout, with the error handled.
	//    This is the capability that makes a logger usable as a library.
	f, err := os.OpenFile("stacklog-example.log", os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		os.Stderr.WriteString("could not open the log file: " + err.Error() + "\n")
		return
	}
	defer f.Close()

	fileLogger := base.WithWriter(f)

	if err := fileLogger.Error("this line goes to the file, not to stdout"); err != nil {
		os.Stderr.WriteString("write failed: " + err.Error() + "\n")
	}

	os.Stdout.WriteString("wrote a line to stacklog-example.log\n")

	// 6. Types that AppendAny does not handle are written as <unsupported>.
	//    float64, time.Duration and error are the common ones missing.
	custom.WithParams("float", 3.14, "handled", 5).Warn("float64 falls through to the default branch")

	// 7. Fatal writes the line and exits with status 1. Deferred functions do not
	//    run, so the file above is not flushed by the deferred Close if the writer
	//    were buffered. Keep this last.
	custom.Fatal("the last line")
}
