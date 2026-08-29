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

	CallbackCommandUpdateWeek     = "update_week"
	CallbackCommandUpdateGroup    = "update_group"
	CallbackCommandUpdateTeacher  = "update_teacher"
	CallbackCommandUpdateTomorrow = "update_tomorrow"
	CallbackCommandUpdateToday    = "update_today"
)

// UpdateKind identifies the schedule view an update button refreshes.
type UpdateKind string

const (
	UpdateKindWeek     UpdateKind = "week"
	UpdateKindToday    UpdateKind = "today"
	UpdateKindTomorrow UpdateKind = "tomorrow"
)

// CallbackCommand returns the callback command prefix for the kind.
func (k UpdateKind) CallbackCommand() string {
	return "update_" + string(k)
}
