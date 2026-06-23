package model

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
