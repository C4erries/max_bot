package logger

import (
	stdlog "log"
	"regexp"
	"strings"

	"github.com/rs/zerolog"
)

const redactedPlaceholder = "<redacted>"

var accessTokenPattern = regexp.MustCompile(`(?i)access_token=[^&\s"]+`)

type stdlogBridge struct {
	log   zerolog.Logger
	token string
}

// RedirectStdLogger routes the standard library logger output through zerolog and
// redacts sensitive data (e.g. bot tokens) before it hits stdout/stderr.
func RedirectStdLogger(logger zerolog.Logger, token string) {
	bridge := &stdlogBridge{
		log:   logger.With().Str("component", "stdlog").Logger(),
		token: token,
	}

	stdlog.SetFlags(0)
	stdlog.SetPrefix("")
	stdlog.SetOutput(bridge)
}

func (b *stdlogBridge) Write(p []byte) (int, error) {
	msg := strings.TrimSpace(string(p))
	if msg == "" {
		return len(p), nil
	}

	sanitized := redactSensitiveData(msg, b.token)
	b.log.Warn().Str("source", "stdlib").Msg(sanitized)
	return len(p), nil
}

func redactSensitiveData(text, token string) string {
	result := accessTokenPattern.ReplaceAllStringFunc(text, func(match string) string {
		parts := strings.SplitN(match, "=", 2)
		if len(parts) != 2 {
			return "access_token=" + redactedPlaceholder
		}
		return parts[0] + "=" + redactedPlaceholder
	})

	if token != "" {
		result = strings.ReplaceAll(result, token, redactedPlaceholder)
	}

	return result
}
