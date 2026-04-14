package routes

import (
"fmt"
"net/http"
"strings"

"github.com/chifamba/vashandi/vashandi/backend/db/models"
"github.com/go-chi/chi/v5"
"gorm.io/gorm"
)

func OrgChartSVGHandler(db *gorm.DB) http.HandlerFunc {
return func(w http.ResponseWriter, r *http.Request) {
companyID := chi.URLParam(r, "companyId")
style := r.URL.Query().Get("style")

var agents []models.Agent
db.WithContext(r.Context()).Where("company_id = ?", companyID).Find(&agents)

nodeW, nodeH := 200, 80
hGap, vGap := 20, 60

// Build adjacency: id -> children
childMap := map[string][]string{}
roots := []string{}
idToAgent := map[string]models.Agent{}
for _, a := range agents {
idToAgent[a.ID] = a
if a.ReportsTo == nil || *a.ReportsTo == "" {
roots = append(roots, a.ID)
} else {
childMap[*a.ReportsTo] = append(childMap[*a.ReportsTo], a.ID)
}
}

// BFS layout
type pos struct{ x, y int }
positions := map[string]pos{}
colIdx := map[int]int{} // level -> next col index
var bfs func(id string, level int)
bfs = func(id string, level int) {
col := colIdx[level]
colIdx[level]++
x := col*(nodeW+hGap) + 10
y := level*(nodeH+vGap) + 10
positions[id] = pos{x, y}
for _, child := range childMap[id] {
bfs(child, level+1)
}
}
for i, root := range roots {
colIdx[0] = i
bfs(root, 0)
}

maxX, maxY := 400, 200
for _, p := range positions {
if p.x+nodeW+10 > maxX {
maxX = p.x + nodeW + 10
}
if p.y+nodeH+10 > maxY {
maxY = p.y + nodeH + 10
}
}

bgColor, nodeColor, textColor := "white", "#e0e7ff", "#1e1b4b"
if style == "nebula" {
bgColor, nodeColor, textColor = "#0f0c29", "#6d28d9", "#f3f4f6"
}

var sb strings.Builder
sb.WriteString(fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="%d">`, maxX, maxY))
sb.WriteString(fmt.Sprintf(`<rect width="100%%" height="100%%" fill="%s"/>`, bgColor))

// Edges
for id, p := range positions {
for _, childID := range childMap[id] {
if cp, ok := positions[childID]; ok {
sb.WriteString(fmt.Sprintf(
`<line x1="%d" y1="%d" x2="%d" y2="%d" stroke="%s" stroke-width="1.5"/>`,
p.x+nodeW/2, p.y+nodeH,
cp.x+nodeW/2, cp.y,
textColor,
))
}
}
}

// Nodes
for id, p := range positions {
agent := idToAgent[id]
name := agent.Name
if len(name) > 20 {
name = name[:20] + "..."
}
sb.WriteString(fmt.Sprintf(
`<rect x="%d" y="%d" width="%d" height="%d" rx="8" fill="%s" stroke="%s"/>`,
p.x, p.y, nodeW, nodeH, nodeColor, textColor,
))
sb.WriteString(fmt.Sprintf(
`<text x="%d" y="%d" fill="%s" font-size="13" text-anchor="middle" dominant-baseline="middle">%s</text>`,
p.x+nodeW/2, p.y+nodeH/2, textColor, htmlEscape(name),
))
}

sb.WriteString(`</svg>`)
w.Header().Set("Content-Type", "image/svg+xml")
w.Write([]byte(sb.String()))
}
}

func htmlEscape(s string) string {
s = strings.ReplaceAll(s, "&", "&amp;")
s = strings.ReplaceAll(s, "<", "&lt;")
s = strings.ReplaceAll(s, ">", "&gt;")
return s
}

// OrgChartPNGHandler returns the org chart as a PNG redirect to SVG fallback
func OrgChartPNGHandler(db *gorm.DB) http.HandlerFunc {
	return OrgChartSVGHandler(db)
}
