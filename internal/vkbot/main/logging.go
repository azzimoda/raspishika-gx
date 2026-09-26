package vkbot

import (
	"context"

	"github.com/rs/zerolog/log"
)

type contextKey string

const (
	keyError          contextKey = "vk_error"
	keyNoLogFlag      contextKey = "vk_default_handler"
	keyGroupOrTeacher contextKey = "vk_group_or_teacher"
	keyCached         contextKey = "vk_cached"
)

// addHandlerCtxErr adds an error to the handler error context.
func addHandlerCtxErr(ctx context.Context, err error) {
	handlerErrs, ok := ctx.Value(keyError).(*[]error)
	if !ok {
		log.Warn().Err(err).Msg("Error context not found")
		return
	}
	if err != nil {
		*handlerErrs = append(*handlerErrs, err)
	}
}

// setNoLogFlag flags the update for skipping statistics, mirroring the
// Telegram default handler that ignores unhandled messages.
func setNoLogFlag(ctx context.Context) {
	noLogFlag, ok := ctx.Value(keyNoLogFlag).(*bool)
	if ok {
		*noLogFlag = true
	} else {
		log.Warn().Msg("Failed to set no-log handler flag")
	}
}

func setGroupOrTeacherAndCached(ctx context.Context, groupOrTeacher string, cached bool) {
	if ctx == nil {
		return
	}
	groupOrTeacherValue := ctx.Value(keyGroupOrTeacher)
	if p, ok := groupOrTeacherValue.(*string); ok {
		*p = groupOrTeacher
	} else {
		log.Warn().Msg("groupOrTeacher context not found")
	}

	cachedValue := ctx.Value(keyCached)
	if p, ok := cachedValue.(*bool); ok {
		*p = cached
	} else {
		log.Warn().Msg("cached context not found")
	}
}

func shortenText(text string, maxLength int) string {
	if len(text) > maxLength {
		return text[:maxLength-2] + "…"
	}
	return text
}
