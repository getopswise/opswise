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
	"github.com/getopswise/opswise/app/internal/runner"
	"github.com/getopswise/opswise/app/web/templates"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

func main() {
	dbPath := os.Getenv("OPSWISE_DB")
	if dbPath == "" {
		dbPath = "opswise.db"
	}

	deployDir := os.Getenv("OPSWISE_DEPLOY_DIR")
	if deployDir == "" {
		// Default: ../deploy relative to the working directory
		deployDir = filepath.Join("..", "deploy")
	}

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
	productHandler := api.NewProductHandler(queries, deploySvc)
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
	r.Post("/hosts", hostHandler.Create)
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

	addr := ":8080"
	log.Printf("Opswise Deploy starting on %s", addr)
	if err := http.ListenAndServe(addr, r); err != nil {
		log.Fatalf("server error: %v", err)
	}
}
