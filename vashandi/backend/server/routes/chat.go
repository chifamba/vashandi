package routes
import (
	"bytes"
	"encoding/json"
	"net/http"
	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"
)
func RegisterChatRoutes(r chi.Router, db *gorm.DB) {
	r.Post("/ceo/chat", CeoChatIngestionHandler(db))
}
func CeoChatIngestionHandler(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		companyID := chi.URLParam(r, "companyId")
		var req struct{ Message string `json:"message"` }
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		obReq := map[string]interface{}{"records": []map[string]interface{}{{"text": req.Message, "metadata": map[string]string{"type": "strategy", "source": "ceo_chat"}}}}
		obBody, _ := json.Marshal(obReq)
		obURL := "http://localhost:3101/v1/namespaces/" + companyID + "/memories"
		httpReq, _ := http.NewRequest("POST", obURL, bytes.NewBuffer(obBody))
		httpReq.Header.Set("Content-Type", "application/json")
		client := &http.Client{}
		client.Do(httpReq)
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"status": "ingested"})
	}
}
