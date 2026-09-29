package controller

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestInvoiceApplicationDayUsesMonthEndWhenConfiguredDayIsMissing(t *testing.T) {
	tests := []struct {
		name          string
		configuredDay int
		date          time.Time
		expectedDay   int
	}{
		{name: "31st in February non leap year", configuredDay: 31, date: time.Date(2027, time.February, 12, 0, 0, 0, 0, time.UTC), expectedDay: 28},
		{name: "31st in February leap year", configuredDay: 31, date: time.Date(2028, time.February, 12, 0, 0, 0, 0, time.UTC), expectedDay: 29},
		{name: "31st in April", configuredDay: 31, date: time.Date(2027, time.April, 12, 0, 0, 0, 0, time.UTC), expectedDay: 30},
		{name: "31st in May", configuredDay: 31, date: time.Date(2027, time.May, 12, 0, 0, 0, 0, time.UTC), expectedDay: 31},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.expectedDay, invoiceApplicationDayForMonth(test.configuredDay, test.date))
		})
	}
}
