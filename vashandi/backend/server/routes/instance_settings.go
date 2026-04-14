package routes

import (
"encoding/json"
"net/http"

"github.com/chifamba/vashandi/vashandi/backend/db/models"
"gorm.io/datatypes"
"gorm.io/gorm"
)

func GetGeneralSettingsHandler(db *gorm.DB) http.HandlerFunc {
return func(w http.ResponseWriter, r *http.Request) {
var setting models.InstanceSetting
if err := db.WithContext(r.Context()).Where("singleton_key = ?", "default").FirstOrCreate(&setting).Error; err != nil {
http.Error(w, err.Error(), http.StatusInternalServerError)
return
}
w.Header().Set("Content-Type", "application/json")
json.NewEncoder(w).Encode(map[string]interface{}{"general": setting.General})
}
}

func UpdateGeneralSettingsHandler(db *gorm.DB) http.HandlerFunc {
return func(w http.ResponseWriter, r *http.Request) {
var setting models.InstanceSetting
db.WithContext(r.Context()).Where("singleton_key = ?", "default").FirstOrCreate(&setting)
var body json.RawMessage
if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
http.Error(w, err.Error(), http.StatusBadRequest)
return
}
setting.General = datatypes.JSON(body)
db.WithContext(r.Context()).Save(&setting)
w.Header().Set("Content-Type", "application/json")
json.NewEncoder(w).Encode(map[string]interface{}{"general": setting.General})
}
}

func GetExperimentalSettingsHandler(db *gorm.DB) http.HandlerFunc {
return func(w http.ResponseWriter, r *http.Request) {
var setting models.InstanceSetting
db.WithContext(r.Context()).Where("singleton_key = ?", "default").FirstOrCreate(&setting)
w.Header().Set("Content-Type", "application/json")
json.NewEncoder(w).Encode(map[string]interface{}{"experimental": setting.Experimental})
}
}

func UpdateExperimentalSettingsHandler(db *gorm.DB) http.HandlerFunc {
return func(w http.ResponseWriter, r *http.Request) {
var setting models.InstanceSetting
db.WithContext(r.Context()).Where("singleton_key = ?", "default").FirstOrCreate(&setting)
var body json.RawMessage
if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
http.Error(w, err.Error(), http.StatusBadRequest)
return
}
setting.Experimental = datatypes.JSON(body)
db.WithContext(r.Context()).Save(&setting)
w.Header().Set("Content-Type", "application/json")
json.NewEncoder(w).Encode(map[string]interface{}{"experimental": setting.Experimental})
}
}
