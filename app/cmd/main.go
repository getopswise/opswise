package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"path/filepath"

	"github.com/getopswise/opswise/app/internal/api"
	"github.com/getopswise/opswise/app/internal/db"
	"github.com/getopswise/opswise/app/internal/db/dbq"
	"github.com/getopswise/opswise/app/internal/models"
	"github.com/getopswise/opswise/app/internal/runner"
	"github.com/getopswise/opswise/app/web/templates"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"gopkg.in/yaml.v3"
)

func loadConfig() models.AppConfig {
	cfg := models.AppConfig{
		DBPath:    "opswise.db",
		Port:      ":8080",
		DeployDir: filepath.Join("..", "deploy"),
	}

	// Load from config.yaml if it exists
	data, err := os.ReadFile("config.yaml")
	if err == nil {
		yaml.Unmarshal(data, &cfg)
	}

	// Environment variables override config.yaml
	if v := os.Getenv("OPSWISE_DB"); v != "" {
		cfg.DBPath = v
	}
	if v := os.Getenv("OPSWISE_PORT"); v != "" {
		cfg.Port = ":" + v
	}
	if v := os.Getenv("OPSWISE_DEPLOY_DIR"); v != "" {
		cfg.DeployDir = v
	}

	return cfg
}

func main() {
	cfg := loadConfig()

	dbPath := cfg.DBPath
	deployDir := cfg.DeployDir

	database, err := db.Open(dbPath)
	if err != nil {
		log.Fatalf("failed to open database: %v", err)
	}
	defer database.Close()

	queries := dbq.New(database)

	// Seed products and stacks
	db.Seed(context.Background(), queries)

	deploySvc := runner.NewDeployService(queries, deployDir)

	hostHandler := api.NewHostHandler(queries)
	productHandler := api.NewProductHandler(queries, deploySvc, deployDir)
	stackHandler := api.NewStackHandler(queries, deploySvc)
	deploymentHandler := api.NewDeploymentHandler(queries, deploySvc)
	settingsHandler := api.NewSettingsHandler(queries)

	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	// Static files
	fs := http.FileServer(http.Dir("web/static"))
	r.Handle("/static/*", http.StripPrefix("/static/", fs))

	// Dashboard
	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		hostCount, _ := queries.CountHosts(r.Context())
		productCount, _ := queries.CountProducts(r.Context())
		deploymentCount, _ := queries.CountDeployments(r.Context())
		templates.Dashboard(hostCount, productCount, deploymentCount).Render(r.Context(), w)
	})

	// Hosts
	r.Get("/hosts", hostHandler.List)
	r.Get("/hosts/{id}", hostHandler.Detail)
	r.Post("/hosts", hostHandler.Create)
	r.Post("/hosts/{id}/test", hostHandler.TestConnection)
	r.Delete("/hosts/{id}", hostHandler.Delete)

	// Products
	r.Get("/products", productHandler.List)
	r.Get("/products/{name}", productHandler.Detail)
	r.Post("/products/{name}/deploy", productHandler.Deploy)

	// Stacks
	r.Get("/stacks", stackHandler.List)
	r.Get("/stacks/{name}", stackHandler.Detail)
	r.Post("/stacks/{name}/deploy", stackHandler.Deploy)

	// Deployments
	r.Get("/deployments", deploymentHandler.List)
	r.Get("/deployments/{id}", deploymentHandler.Detail)
	r.Get("/deployments/{id}/log", deploymentHandler.LogStream)

	// Settings
	r.Get("/settings", settingsHandler.Page)
	r.Post("/settings", settingsHandler.Save)

	addr := cfg.Port
	log.Printf("Opswise Deploy starting on %s", addr)
	if err := http.ListenAndServe(addr, r); err != nil {
		log.Fatalf("server error: %v", err)
	}
}
