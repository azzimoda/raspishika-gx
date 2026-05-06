package model

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"time"
)

type GroupID string

func (i GroupID) String() string { return string(i) }

type DepartmentID string

func (i DepartmentID) String() string { return string(i) }

// GroupName is a string representing name of a student group.
type GroupName string

const GroupRegexp = `^([\w\p{Cyrillic}]{3,5})[- ]*(\d{2})[- ]*\(?(9|11)\)?[- ]*(\d)$`

var GroupRE = regexp.MustCompile(GroupRegexp)

var ErrInvalidGroupNameFormat = errors.New("string does not match the group name format")

// ValidateFormat determines whether the given string can be formatted into a valid group name,
// and if it can, returns valid group name, else returns an error.
// It uses regexp provided by the constant [GroupRegexp].
//
// Important: the function doesn't validate case, i.e. if string "иСпТ-22-(9)-2" is given, the result is the same.
func (n GroupName) ValidateFormat() (GroupName, error) {
	if !GroupRE.MatchString(string(n)) {
		return n, fmt.Errorf("%w: '%s'", ErrInvalidGroupNameFormat, n)
	}
	subs := GroupRE.FindStringSubmatch(string(n))
	return GroupName(fmt.Sprintf("%s-%s-(%s)-%s", subs[1], subs[2], subs[3], subs[4])), nil
}

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

type Year int

type Group struct {
	ID             int64          `db:"id"              json:"id"`
	GroupID        GroupID        `db:"group_id"        json:"group_id"`
	DepartmentID   DepartmentID   `db:"department_id"   json:"department_id"`
	GroupName      GroupName      `db:"group_name"      json:"group_name"`
	DepartmentName DepartmentName `db:"department_name" json:"department_name"`
	Year           Year           `db:"year"            json:"year"`
	CreatedAt      time.Time      `db:"created_at"      json:"created_at"`
	UpdatedAt      time.Time      `db:"updated_at"      json:"updated_at"`
}

// ApplicationYear returns the year of the group's application.
//
// The application year is indicated in group's name. For example, group "ИСПт-22-(9)-2" has application year 2022.
func (g *Group) ApplicationYear() int {
	_, year, _, _, err := g.GroupName.Parse()
	_ = err
	return year
}
