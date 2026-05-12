package botutil

const (
	CallbackCommandDelete       = "delete"
	CallbackCommandDeleteConfig = "delete_config"

	CallbackCommandConfigGroup     = "config_group"
	CallbackCommandConfigDailyTime = "config_daily_time"
	CallbackCommandDailyOff        = "daily_off"
	CallbackCommandConfigReminder  = "config_reminder"
	CallbackCommandConfigChange    = "config_change"
	CallbackCommandConfigDarkMode  = "config_darkmode"
	CallbackCommandSetAccess       = "set_access"

	CallbackCommandSelectDepartment = "select_department"
	CallbackCommandSelectTeacher    = "select_teacher"

	CallbackCommandUpdateGroup    = "update_group"
	CallbackCommandUpdateTeacher  = "update_teacher"
	CallbackCommandUpdateTomorrow = "update_tomorrow"
	// Deprecated: Use [CallbackCommandUpdateToday] instead.
	// TODO: Remove constant [CallbackCommandUpdateLeft] and handlers dependencies after 2026-06-01
	CallbackCommandUpdateLeft  = "update_left"
	CallbackCommandUpdateToday = "update_today"
)
