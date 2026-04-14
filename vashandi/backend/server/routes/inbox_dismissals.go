package routes

import (
"encoding/json"
"net/http"
"time"

"github.com/chifamba/vashandi/vashandi/backend/db/models"
"github.com/go-chi/chi/v5"
"gorm.io/gorm"
)

func ListInboxDismissalsHandler(db *gorm.DB) http.HandlerFunc {
return func(w http.ResponseWriter, r *http.Request) {
companyID := chi.URLParam(r, "companyId")
userID := r.URL.Query().Get("userId")
var dismissals []models.InboxDismissal
q := db.WithContext(r.Context()).Where("company_id = ?", companyID)
if userID != "" {
q = q.Where("user_id = ?", userID)
}
q.Find(&dismissals)
if dismissals == nil {
dismissals = []models.InboxDismissal{}
}
w.Header().Set("Content-Type", "application/json")
json.NewEncoder(w).Encode(dismissals)
}
}

func CreateInboxDismissalHandler(db *gorm.DB) http.HandlerFunc {
return func(w http.ResponseWriter, r *http.Request) {
companyID := chi.URLParam(r, "companyId")
var d models.InboxDismissal
if err := json.NewDecoder(r.Body).Decode(&d); err != nil {
http.Error(w, err.Error(), http.StatusBadRequest)
return
}
d.CompanyID = companyID
db.WithContext(r.Context()).
Where(models.InboxDismissal{CompanyID: companyID, UserID: d.UserID, ItemKey: d.ItemKey}).
Assign(models.InboxDismissal{DismissedAt: time.Now()}).
FirstOrCreate(&d)
w.Header().Set("Content-Type", "application/json")
w.WriteHeader(http.StatusCreated)
json.NewEncoder(w).Encode(d)
}
}
