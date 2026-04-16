package services

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/chifamba/vashandi/vashandi/backend/db/models"
	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

type BackupRetention struct {
	DailyDays     int `json:"dailyDays"`
	WeeklyWeeks   int `json:"weeklyWeeks"`
	MonthlyMonths int `json:"monthlyMonths"`
}

type GeneralSettings struct {
	CensorUsernameInLogs          bool            `json:"censorUsernameInLogs"`
	KeyboardShortcuts             bool            `json:"keyboardShortcuts"`
	FeedbackDataSharingPreference string          `json:"feedbackDataSharingPreference"`
	BackupRetention               BackupRetention `json:"backupRetention"`
	Storage                       map[string]any  `json:"storage,omitempty"`
}

type ExperimentalSettings struct {
	EnableIsolatedWorkspaces     bool `json:"enableIsolatedWorkspaces"`
	AutoRestartDevServerWhenIdle bool `json:"autoRestartDevServerWhenIdle"`
}

var DefaultGeneralSettings = GeneralSettings{
	CensorUsernameInLogs:          false,
	KeyboardShortcuts:             false,
	FeedbackDataSharingPreference: "prompt",
	BackupRetention: BackupRetention{
		DailyDays:     7,
		WeeklyWeeks:   4,
		MonthlyMonths: 1,
	},
}

var DefaultExperimentalSettings = ExperimentalSettings{
	EnableIsolatedWorkspaces:     false,
	AutoRestartDevServerWhenIdle: false,
}

type InstanceSettingsService struct {
	db *gorm.DB
}

func NewInstanceSettingsService(db *gorm.DB) *InstanceSettingsService {
	return &InstanceSettingsService{db: db}
}

func (s *InstanceSettingsService) GetGeneral(ctx context.Context) (GeneralSettings, error) {
	setting, err := s.loadOrCreate(ctx)
	if err != nil {
		return DefaultGeneralSettings, err
	}
	var res GeneralSettings
	if err := s.decode(setting.General, &res, DefaultGeneralSettings); err != nil {
		return DefaultGeneralSettings, err
	}
	return res, nil
}

func (s *InstanceSettingsService) GetExperimental(ctx context.Context) (ExperimentalSettings, error) {
	setting, err := s.loadOrCreate(ctx)
	if err != nil {
		return DefaultExperimentalSettings, err
	}
	var res ExperimentalSettings
	if err := s.decode(setting.Experimental, &res, DefaultExperimentalSettings); err != nil {
		return DefaultExperimentalSettings, err
	}
	return res, nil
}

func (s *InstanceSettingsService) UpdateGeneral(ctx context.Context, patch map[string]any) (GeneralSettings, error) {
	setting, err := s.loadOrCreate(ctx)
	if err != nil {
		return DefaultGeneralSettings, err
	}
	setting.General = s.merge(setting.General, patch)
	if err := s.db.WithContext(ctx).Save(setting).Error; err != nil {
		return DefaultGeneralSettings, err
	}
	var res GeneralSettings
	s.decode(setting.General, &res, DefaultGeneralSettings)
	return res, nil
}

func (s *InstanceSettingsService) UpdateExperimental(ctx context.Context, patch map[string]any) (ExperimentalSettings, error) {
	setting, err := s.loadOrCreate(ctx)
	if err != nil {
		return DefaultExperimentalSettings, err
	}
	setting.Experimental = s.merge(setting.Experimental, patch)
	if err := s.db.WithContext(ctx).Save(setting).Error; err != nil {
		return DefaultExperimentalSettings, err
	}
	var res ExperimentalSettings
	s.decode(setting.Experimental, &res, DefaultExperimentalSettings)
	return res, nil
}

func (s *InstanceSettingsService) loadOrCreate(ctx context.Context) (*models.InstanceSetting, error) {
	var setting models.InstanceSetting
	if err := s.db.WithContext(ctx).Where("singleton_key = ?", "default").First(&setting).Error; err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, err
		}
		setting = models.InstanceSetting{
			ID:           uuid.NewString(),
			SingletonKey: "default",
			General:      datatypes.JSON([]byte(`{}`)),
			Experimental: datatypes.JSON([]byte(`{}`)),
		}
		if err := s.db.WithContext(ctx).Create(&setting).Error; err != nil {
			return nil, err
		}
	}
	return &setting, nil
}

func (s *InstanceSettingsService) decode(raw datatypes.JSON, target any, defaults any) error {
	// Start with defaults
	defaultBody, _ := json.Marshal(defaults)
	_ = json.Unmarshal(defaultBody, target)

	if len(raw) == 0 {
		return nil
	}
	// Overlay persisted values
	if err := json.Unmarshal(raw, target); err != nil {
		return err
	}
	return nil
}

func (s *InstanceSettingsService) merge(raw datatypes.JSON, patch map[string]any) datatypes.JSON {
	var current map[string]any
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &current)
	}
	if current == nil {
		current = make(map[string]any)
	}
	for key, value := range patch {
		current[key] = value
	}
	body, _ := json.Marshal(current)
	return datatypes.JSON(body)
}
