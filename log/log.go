package log

import (
	"io"
	"log/slog"

	"github.com/go-logr/logr"

	"github.com/abrander/colorjson"
)

var Verbosity int
var Pretty bool

func NewLogger(writer io.Writer) logr.Logger {
	var handler slog.Handler
	if Pretty {
		writer = colorjson.NewEncoder(writer, &colorjson.Settings{
			EndWithNewline: true,
			Newlines:       true,
			Indent:         "  ",
			Separator:      " ",
			Color:          colorjson.DefaultColors,
			ColorMode:      colorjson.Always,
		})
	}
	handler = slog.NewJSONHandler(writer, &slog.HandlerOptions{
		Level: slog.Level(-Verbosity),
	})
	return logr.FromSlogHandler(handler)
}
