package strategy

import "time"

func nowUTC() time.Time {
	return time.Now().UTC().Truncate(time.Second)
}
