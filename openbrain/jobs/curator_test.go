package jobs
import (
	"testing"
	"github.com/chifamba/vashandi/openbrain/db/models"
	"github.com/stretchr/testify/assert"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)
func TestHandleCuratorProposal(t *testing.T) {
	db, _ := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	db.AutoMigrate(&models.Memory{}, &models.Namespace{}, &models.Proposal{})
	db.Create(&models.Memory{ID: "m1", NamespaceID: "ns1", Text: "fact 1"})
	db.Create(&models.Memory{ID: "m2", NamespaceID: "ns1", Text: "fact 2"})
	job := &CuratorJob{DB: db}
	err := job.HandleCuratorProposal([]byte(`{"namespace_id": "ns1"}`))
	assert.NoError(t, err)
	var proposals []models.Proposal
	db.Find(&proposals)
	assert.Equal(t, 1, len(proposals))
}
