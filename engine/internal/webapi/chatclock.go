package webapi

import (
	"time"

	"tns65/internal/turn"
)

// chatClock — часы сервера хода: момент последней активности, порог ночи и
// прогон паузы (returnStage). e2e-хук подменяет их фейковыми часами через
// SetE2EClock (файл с тегом e2e); в проде — системные часы.
var chatClock turn.ClockSource = wallChatClock{}

// wallChatClock — системные часы по умолчанию.
type wallChatClock struct{}

func (wallChatClock) Now() time.Time                         { return time.Now() }
func (wallChatClock) After(d time.Duration) <-chan time.Time { return time.After(d) }
