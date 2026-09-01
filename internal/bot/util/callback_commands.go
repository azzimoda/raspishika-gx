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

	CallbackCommandBroadcast           = "broadcast"
	CallbackCommandBroadcastAll        = "broadcast_all"
	CallbackCommandBroadcastPriv       = "broadcast_priv"
	CallbackCommandBroadcastGroupChats = "broadcast_group_chats"
	CallbackCommandBroadcastByGroup    = "broadcast_by_group"
	CallbackCommandBroadcastDept       = "broadcast_dept"
	CallbackCommandBroadcastActive     = "broadcast_active"
	CallbackCommandBroadcastEdit       = "broadcast_edit"
	CallbackCommandBroadcastConfirm    = "broadcast_confirm"
	CallbackCommandBroadcastCancel     = "broadcast_cancel"
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
