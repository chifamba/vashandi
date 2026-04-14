package routes

import (
"encoding/json"
"net/http"
"sync"
"time"

"github.com/go-chi/chi/v5"
)

type InboxDismissal struct {
ID          string    `json:"id"`
CompanyID   string    `json:"companyId"`
UserID      string    `json:"userId"`
ItemKey     string    `json:"itemKey"`
DismissedAt time.Time `json:"dismissedAt"`
}

var (
dismissalMu    sync.RWMutex
dismissalStore = map[string]InboxDismissal{}
)

func ListInboxDismissalsHandler() http.HandlerFunc {
return func(w http.ResponseWriter, r *http.Request) {
companyID := chi.URLParam(r, "companyId")
userID := r.URL.Query().Get("userId")
dismissalMu.RLock()
defer dismissalMu.RUnlock()
var result []InboxDismissal
for _, d := range dismissalStore {
if d.CompanyID == companyID && (userID == "" || d.UserID == userID) {
result = append(result, d)
}
}
if result == nil {
result = []InboxDismissal{}
}
w.Header().Set("Content-Type", "application/json")
json.NewEncoder(w).Encode(result)
}
}

func CreateInboxDismissalHandler() http.HandlerFunc {
return func(w http.ResponseWriter, r *http.Request) {
companyID := chi.URLParam(r, "companyId")
var d InboxDismissal
if err := json.NewDecoder(r.Body).Decode(&d); err != nil {
http.Error(w, err.Error(), http.StatusBadRequest)
return
}
d.CompanyID = companyID
d.DismissedAt = time.Now()
key := companyID + ":" + d.UserID + ":" + d.ItemKey
dismissalMu.Lock()
dismissalStore[key] = d
dismissalMu.Unlock()
w.Header().Set("Content-Type", "application/json")
w.WriteHeader(http.StatusCreated)
json.NewEncoder(w).Encode(d)
}
}
