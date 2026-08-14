package model

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
)

// Group represents a study group with all identifiers required to build
// its schedule URL.
type Group struct {
	GroupID        string    `json:"group_id" example:"205"`
	DepartmentID   string    `json:"department_id" example:"15"`
	GroupName      GroupName `json:"group_name" example:"ИСПт-22-(9)-2"`
	DepartmentName string    `json:"department_name" example:"Отделение СОНХ"`
	Year           int       `json:"year" example:"2026"`
}

// GroupName is a normalized study group name, e.g. "ИСПт-22-(9)-2".
type GroupName string

// GroupRegexp matches group names in their base format,
// where the base is either 9 or 11.
const GroupRegexp = `^([\w\p{Cyrillic}]{3,5})[- ]*(\d{2})[- ]*\(?(9|11)\)?[- ]*(\d)$`

// GroupRE is the compiled form of GroupRegexp.
var GroupRE = regexp.MustCompile(GroupRegexp)

// ErrInvalidGroupNameFormat is returned when a string does not match
// the group name format.
var ErrInvalidGroupNameFormat = errors.New("string does not match the group name format")

// ValidateFormat checks the group name against GroupRegexp and returns
// its canonical "XXX-YY-(Z)-N" form.
func (n GroupName) ValidateFormat() (GroupName, error) {
	if !GroupRE.MatchString(string(n)) {
		return n, fmt.Errorf("%w: '%s'", ErrInvalidGroupNameFormat, n)
	}
	subs := GroupRE.FindStringSubmatch(string(n))
	return GroupName(fmt.Sprintf("%s-%s-(%s)-%s", subs[1], subs[2], subs[3], subs[4])), nil
}

// Parse splits a group name into its components:
// the name prefix, year of admission, base (9 or 11) and group number.
func (group GroupName) Parse() (name string, year int, base int, n int, err error) {
	if !GroupRE.MatchString(string(group)) {
		return "", 0, 0, 0, fmt.Errorf("%w: '%s'", ErrInvalidGroupNameFormat, group)
	}
	subs := GroupRE.FindStringSubmatch(string(group))
	year, _ = strconv.Atoi(subs[2])
	base, _ = strconv.Atoi(subs[3])
	n, _ = strconv.Atoi(subs[4])
	return subs[1], year, base, n, nil
}
