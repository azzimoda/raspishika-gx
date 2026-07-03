package model

import "time"

type DepartmentName string

func (n DepartmentName) String() string { return string(n) }

type URL string

func (u URL) String() string { return string(u) }

type Department struct {
	ID        int            `db:"id"`
	Name      DepartmentName `db:"name"`
	URL       URL            `db:"url"`
	CreatedAt time.Time      `db:"created_at"`
	UpdatedAt time.Time      `db:"updated_at"`
}

func (d *Department) IsActual(ttl time.Duration) bool { return d.UpdatedAt.Add(ttl).After(time.Now()) }
