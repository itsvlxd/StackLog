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
	//    value falls back to the named preset, which is the package palette when
	//    no preset is given. The arrays are sized by MaxLevel, which is exported.
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

	// 3. Presets. A preset fills only the components left as the zero value, so a
	//    single component override stands against the whole preset. An unknown name
	//    resolves to default.
	preset := stacklog.CreateLogger(stacklog.Config{
		Timestamp: true,
		Style: stacklog.Style{
			Preset: stacklog.PresetBadge,
			Level: [stacklog.MaxLevel]stacklog.CompStyle{
				stacklog.NewStyle().Foreground("#FFFFFF").Background("#D75F5F"),
			},
		},
	})

	preset.Debug("level 0 is the caller's override, not the preset")
	preset.Error("badge: the level is a block, with padding inside it")

	// Plain sets nothing, so no escape sequence is written at all. This is the one
	// that makes a log file readable without a terminal.
	plain := stacklog.CreateLogger(stacklog.Config{Style: stacklog.Style{Preset: stacklog.PresetPlain}})
	plain.Warn("no colour codes and no reset codes")

	// The prefix is styled through Style.Prefix. Before that field existed the
	// prefix colour was hardcoded, so a preset could not reach it.
	hc := stacklog.CreateLogger(stacklog.Config{Style: stacklog.Style{Preset: stacklog.PresetHighContrast}})
	hc.WithPrefix("http").Error("the prefix colour comes from the preset")

	// 4. Prefix style and the copy pattern. WithParams and WithPrefix return new
	//    loggers, so a base logger can be shared and specialised per call site.
	scoped := custom.WithPrefix("http")
	paired := scoped.WithParams("request_id", 42, "user", "vlad")

	paired.Warn("params on the shared logger")
	scoped.Info("this logger has no params, the original is unchanged")

	// 5. Padding. Left and Right sit inside the coloured block, so a badge with a
	//    background extends over them. Top applies only at the start of a line,
	//    Bottom only after the line ends.
	badge := stacklog.CreateLogger(stacklog.Config{})

	badge.Config.Style.Level[3] = stacklog.NewStyle().
		Padding(stacklog.Padding{Top: 1, Left: 1, Right: 1, Bottom: 2}).
		Background("#D75F5F").
		Foreground("#FFFFFF")

	badge.Error("Top puts a blank line before, Bottom puts two after")
	badge.Info("next line, no padding")

	// 6. Writing somewhere other than stdout, with the error handled.
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

	// 7. Types that AppendAny does not handle are written as <unsupported>.
	//    float64, time.Duration and error are the common ones missing.
	custom.WithParams("float", 3.14, "handled", 5).Warn("float64 falls through to the default branch")

	// 8. Fatal writes the line and exits with status 1. Deferred functions do not
	//    run, so the file above is not flushed by the deferred Close if the writer
	//    were buffered. Keep this last.
	custom.Fatal("the last line")
}
