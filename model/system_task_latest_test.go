package model

import (
	"os"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

func TestLatestSystemTasksDatabaseMatrix(t *testing.T) {
	for _, dialect := range []string{"sqlite", "mysql", "postgres"} {
		t.Run(dialect, func(t *testing.T) {
			var driver gorm.Dialector
			switch dialect {
			case "sqlite":
				driver = sqlite.Open(":memory:")
			case "mysql":
				dsn := os.Getenv("TEST_MYSQL_DSN")
				if dsn == "" {
					t.Skip("TEST_MYSQL_DSN not configured")
				}
				driver = mysql.Open(dsn)
			case "postgres":
				dsn := os.Getenv("TEST_POSTGRES_DSN")
				if dsn == "" {
					t.Skip("TEST_POSTGRES_DSN not configured")
				}
				driver = postgres.Open(dsn)
			}
			db, err := gorm.Open(driver, &gorm.Config{NamingStrategy: schema.NamingStrategy{TablePrefix: "latest_task_test_"}})
			require.NoError(t, err)
			sqlDB, err := db.DB()
			require.NoError(t, err)
			sqlDB.SetMaxOpenConns(1)
			require.NoError(t, db.AutoMigrate(&SystemTask{}, &SystemTaskLock{}))
			previousDB := DB
			DB = db
			t.Cleanup(func() {
				DB = previousDB
				require.NoError(t, db.Migrator().DropTable(&SystemTaskLock{}, &SystemTask{}))
				require.NoError(t, sqlDB.Close())
			})

			versionQuery := "SELECT version()"
			if dialect == "sqlite" {
				versionQuery = "SELECT sqlite_version()"
			}
			var version string
			require.NoError(t, db.Raw(versionQuery).Scan(&version).Error)
			t.Logf("database: %s %s", dialect, version)
			fixtures := []SystemTask{
				{ID: 10, TaskID: "scheduled_old", Type: "scheduled", Status: SystemTaskStatusSucceeded, CreatedAt: 9000, UpdatedAt: 9000},
				{ID: 20, TaskID: "scheduled_pending", Type: "scheduled", Status: SystemTaskStatusPending, CreatedAt: 8000, UpdatedAt: 8000},
				{ID: 30, TaskID: "scheduled_running", Type: "scheduled", Status: SystemTaskStatusRunning, CreatedAt: 7000, UpdatedAt: 7000},
				{ID: 40, TaskID: "scheduled_new", Type: "scheduled", Status: SystemTaskStatusFailed, CreatedAt: 100, UpdatedAt: 100, Payload: `{"source":"fixture"}`, State: `{"progress":3}`, Result: `{"done":false}`, Error: "preserved error", LockedBy: "previous-runner"},
				{ID: 50, TaskID: "other_newer", Type: "other", Status: SystemTaskStatusSucceeded, CreatedAt: 10000, UpdatedAt: 10000},
				{ID: 60, TaskID: "legacy_empty_type", Type: "", Status: SystemTaskStatusPending, CreatedAt: 1, UpdatedAt: 1},
			}
			require.NoError(t, db.Create(&fixtures).Error)
			lock := SystemTaskLock{Type: "scheduled", TaskID: "scheduled_running", LockedBy: "lease-owner", LockedUntil: 12345, UpdatedAt: 2345}
			require.NoError(t, db.Create(&lock).Error)

			for _, status := range []SystemTaskStatus{SystemTaskStatusPending, SystemTaskStatusRunning, SystemTaskStatusSucceeded, SystemTaskStatusFailed} {
				t.Run(string(status), func(t *testing.T) {
					require.NoError(t, db.Model(&SystemTask{}).Where("id = ?", 40).UpdateColumn("status", status).Error)
					tasks, err := GetLatestSystemTasks([]string{"scheduled"})
					require.NoError(t, err)
					require.Len(t, tasks, 1)
					expected := fixtures[3]
					expected.Status = status
					assert.Equal(t, &expected, tasks["scheduled"], "latest means greatest ID, regardless of status or timestamps")
				})
			}

			for _, tc := range []struct {
				name  string
				types []string
				ids   map[string]int64
			}{
				{name: "no enabled types", ids: map[string]int64{}},
				{name: "missing single type", types: []string{"missing"}, ids: map[string]int64{}},
				{name: "literal empty type", types: []string{""}, ids: map[string]int64{"": 60}},
				{name: "multiple enabled types", types: []string{"scheduled", "other", "missing"}, ids: map[string]int64{"scheduled": 40, "other": 50}},
				{name: "duplicate requested types", types: []string{"scheduled", "scheduled"}, ids: map[string]int64{"scheduled": 40}},
			} {
				t.Run(tc.name, func(t *testing.T) {
					tasks, err := GetLatestSystemTasks(tc.types)
					require.NoError(t, err)
					require.NotNil(t, tasks)
					got := make(map[string]int64, len(tasks))
					for taskType, task := range tasks {
						got[taskType] = task.ID
					}
					assert.Equal(t, tc.ids, got)
				})
			}
			var storedLock SystemTaskLock
			require.NoError(t, db.Where("type = ?", lock.Type).First(&storedLock).Error)
			assert.Equal(t, lock, storedLock, "latest lookups must not change leases or pending task state")
			var pending SystemTask
			require.NoError(t, db.First(&pending, 20).Error)
			assert.Equal(t, SystemTaskStatusPending, pending.Status)
		})
	}
}
