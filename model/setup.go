package model

import (
	"errors"
	"sort"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

var ErrSetupCompleted = errors.New("system setup is already completed")

type Setup struct {
	ID            uint   `json:"id" gorm:"primaryKey"`
	Version       string `json:"version" gorm:"type:varchar(50);not null"`
	InitializedAt int64  `json:"initialized_at" gorm:"type:bigint;not null"`
}

func GetSetup() *Setup {
	var setup Setup
	result := DB.Limit(1).Find(&setup)
	if result.Error != nil || result.RowsAffected == 0 {
		return nil
	}
	return &setup
}

// InitializeSystem claims the singleton setup row before creating the root
// account and options. The primary-key claim serializes concurrent setup
// requests across processes on every supported database.
func InitializeSystem(rootUser *User, optionValues map[string]string, version string, initializedAt int64) error {
	err := DB.Transaction(func(tx *gorm.DB) error {
		var setupCount int64
		if err := tx.Model(&Setup{}).Count(&setupCount).Error; err != nil {
			return err
		}
		if setupCount > 0 {
			return ErrSetupCompleted
		}

		setup := Setup{ID: 1, Version: version, InitializedAt: initializedAt}
		if err := tx.Create(&setup).Error; err != nil {
			return err
		}

		var rootCount int64
		if err := tx.Model(&User{}).Where("role = ?", common.RoleRootUser).Count(&rootCount).Error; err != nil {
			return err
		}
		if rootCount == 0 {
			if rootUser == nil {
				return errors.New("root user is required")
			}
			if err := tx.Create(rootUser).Error; err != nil {
				return err
			}
		}

		keys := make([]string, 0, len(optionValues))
		for key := range optionValues {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			option := Option{Key: key}
			if err := tx.FirstOrCreate(&option, Option{Key: key}).Error; err != nil {
				return err
			}
			option.Value = optionValues[key]
			if err := tx.Save(&option).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		if errors.Is(err, ErrSetupCompleted) || GetSetup() != nil {
			return ErrSetupCompleted
		}
		return err
	}

	for key, value := range optionValues {
		if err := updateOptionMap(key, value); err != nil {
			common.SysLog("failed to publish setup option: " + key)
		}
	}
	return nil
}
