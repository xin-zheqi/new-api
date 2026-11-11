package dto

import (
	"fmt"
	"time"
)

const MaxChannelSchedulePeriods = 32

// ChannelSchedule is a daily recurring admission window in an explicit IANA
// timezone. It never changes administrative/health status or interrupts work
// admitted before a window ends. Periods are combined with OR semantics.
type ChannelSchedule struct {
	Timezone string                  `json:"timezone"`
	Periods  []ChannelSchedulePeriod `json:"periods"`
}

type ChannelSchedulePeriod struct {
	Start string `json:"start"`
	End   string `json:"end"`
}

type channelScheduleMinutes struct{ start, end int }

// CompiledChannelSchedule is immutable and safe to share between requests.
// Compile at configuration/cache load, rather than parsing times and loading
// timezone data on each relay request.
type CompiledChannelSchedule struct {
	location *time.Location
	periods  []channelScheduleMinutes
}

func (s *ChannelSchedule) Compile() (*CompiledChannelSchedule, error) {
	if s == nil {
		return nil, nil
	}
	if len(s.Periods) == 0 || len(s.Periods) > MaxChannelSchedulePeriods {
		return nil, fmt.Errorf("channel_schedule requires 1-%d periods", MaxChannelSchedulePeriods)
	}
	// Local depends on a host's environment and can differ across replicas.
	if s.Timezone == "" || s.Timezone == "Local" {
		return nil, fmt.Errorf("channel_schedule requires an explicit IANA timezone")
	}
	location, err := time.LoadLocation(s.Timezone)
	if err != nil {
		return nil, fmt.Errorf("invalid channel_schedule timezone: %w", err)
	}
	compiled := &CompiledChannelSchedule{location: location, periods: make([]channelScheduleMinutes, 0, len(s.Periods))}
	for _, period := range s.Periods {
		start, startOK := channelScheduleMinute(period.Start, false)
		end, endOK := channelScheduleMinute(period.End, true)
		if !startOK || !endOK || start == end {
			return nil, fmt.Errorf("channel_schedule periods require different HH:mm start/end values; only end accepts 24:00")
		}
		compiled.periods = append(compiled.periods, channelScheduleMinutes{start: start, end: end})
	}
	return compiled, nil
}

func channelScheduleMinute(value string, allowEndOfDay bool) (int, bool) {
	if allowEndOfDay && value == "24:00" {
		return 1440, true
	}
	if len(value) != 5 || value[2] != ':' || value[0] < '0' || value[0] > '2' || value[1] < '0' || value[1] > '9' || value[3] < '0' || value[3] > '5' || value[4] < '0' || value[4] > '9' {
		return 0, false
	}
	hour := int(value[0]-'0')*10 + int(value[1]-'0')
	if hour > 23 {
		return 0, false
	}
	return hour*60 + int(value[3]-'0')*10 + int(value[4]-'0'), true
}

// IsActive compares local wall-clock minutes. Both repeated DST hours follow
// the same configured wall-clock window; missing local times do not occur.
// Start is inclusive and end exclusive, including overnight periods.
func (s *CompiledChannelSchedule) IsActive(now time.Time) bool {
	if s == nil {
		return true
	}
	local := now.In(s.location)
	minute := local.Hour()*60 + local.Minute()
	for _, period := range s.periods {
		if period.start < period.end {
			if minute >= period.start && minute < period.end {
				return true
			}
		} else if minute >= period.start || minute < period.end {
			return true
		}
	}
	return false
}
