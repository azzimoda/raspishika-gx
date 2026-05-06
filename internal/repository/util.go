package repository

import (
	"fmt"
	"time"
)

func sqlPeriod(dur time.Duration) string { return fmt.Sprintf("-%d seconds", int(dur.Seconds())) }
