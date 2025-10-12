package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

var templates = map[string]string{
	"repository.go": `package {{.Name}}

import (
	"github.com/jmoiron/sqlx"
)

type Repository interface {
	// TODO: define repository methods
}

type {{.StructNameLower}}Repository struct {
	db *sqlx.DB
}

func New{{.StructName}}Repository(db *sqlx.DB) Repository {
	return &{{.StructNameLower}}Repository{db: db}
}
`,
	"service.go": `package {{.Name}}

import (
	"github.com/jmoiron/sqlx"
)

type Service interface {
	// TODO: define service methods
}

type {{.StructNameLower}}Service struct {
	repo Repository
	db   *sqlx.DB
}

func New{{.StructName}}Service(repo Repository, db *sqlx.DB) Service {
	return &{{.StructNameLower}}Service{repo: repo, db: db}
}
`,
	"route.go": `package {{.Name}}

import (
	"github.com/gofiber/fiber/v2"
	"github.com/jmoiron/sqlx"
)

type Handler interface {
	// TODO: define handler methods
}

type handler struct {
	service Service
	db      *sqlx.DB
}

func NewHandler(app *fiber.App, db *sqlx.DB) Handler {
	repo := New{{.StructName}}Repository(db)
	service := New{{.StructName}}Service(repo, db)
	h := &handler{service: service, db: db}

	routes := app.Group("/api/1.0/{{.Name}}")
	// routes.Get("", h.GetSomething)

	return h
}
`,
	"dto.go": `package {{.Name}}

// TODO: define DTOs here
`,
}

func main() {
	if len(os.Args) < 2 {
		log.Fatal("Usage: go run ./cmd/scaffold/main.go <module-name>")
	}
	name := strings.ToLower(os.Args[1])
	c := cases.Title(language.English)
	structName := c.String(name)

	targetDir := filepath.Join("modules", name)
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		log.Fatal(err)
	}

	for fileName, content := range templates {
		path := filepath.Join(targetDir, fileName)
		if _, err := os.Stat(path); err == nil {
			fmt.Printf("⚠️ %s already exists, skipped\n", path)
			continue
		}
		content = strings.ReplaceAll(content, "{{.Name}}", name)
		content = strings.ReplaceAll(content, "{{.StructName}}", structName)
		content = strings.ReplaceAll(content, "{{.StructNameLower}}", name)

		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			log.Fatal(err)
		}
		fmt.Printf("✅ Created %s\n", path)
	}
}
