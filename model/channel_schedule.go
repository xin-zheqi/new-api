package model

import (
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/tidwall/gjson"
	"gorm.io/gorm"
)

// AfterFind prepares an immutable gate once for each loaded channel. Ordinary
// channels avoid JSON parsing entirely. Invalid stored schedules fail closed
// for this channel without changing its persisted status or other channels.
func (channel *Channel) AfterFind(_ *gorm.DB) error {
	channel.prepareSchedule()
	return nil
}

func (channel *Channel) prepareSchedule() {
	channel.schedulePrepared = true
	channel.scheduleConfigured = false
	channel.schedule = nil
	if channel.Setting == nil || !gjson.Get(*channel.Setting, "channel_schedule").Exists() {
		return
	}
	var settings struct {
		Schedule *dto.ChannelSchedule `json:"channel_schedule"`
	}
	if err := common.UnmarshalJsonStr(*channel.Setting, &settings); err != nil {
		channel.scheduleConfigured = true
		return
	}
	if settings.Schedule == nil {
		return
	}
	channel.scheduleConfigured = true
	channel.schedule, _ = settings.Schedule.Compile()
}

// IsScheduleActiveAt is separate from base Status: entering a window cannot
// revive a manually/health-disabled channel, and automatic tests cannot bypass
// a closed window by writing Status=enabled. Already admitted work is untouched.
func (channel *Channel) IsScheduleActiveAt(now time.Time) bool {
	if channel == nil {
		return false
	}
	if !channel.schedulePrepared {
		// Direct fixtures/admin objects may not have passed through a DB/cache
		// load. Compile a local copy; never lazily mutate a published channel.
		copy := *channel
		copy.prepareSchedule()
		return copy.IsScheduleActiveAt(now)
	}
	return !channel.scheduleConfigured || channel.schedule != nil && channel.schedule.IsActive(now)
}

func (channel *Channel) IsAvailableAt(now time.Time) bool {
	return channel != nil && channel.Status == common.ChannelStatusEnabled && channel.IsScheduleActiveAt(now)
}

func (channel *Channel) IsAvailable() bool {
	return channel.IsAvailableAt(time.Now())
}

// SetScheduleReadState exposes the independent gate to admin clients without
// replacing Status, which existing enable/disable actions still own.
func (channel *Channel) SetScheduleReadState(now time.Time) {
	channel.ScheduleActive = nil
	if !channel.schedulePrepared {
		channel.prepareSchedule()
	}
	if channel.scheduleConfigured {
		active := channel.IsScheduleActiveAt(now)
		channel.ScheduleActive = &active
	}
}
