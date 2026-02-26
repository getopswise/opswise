package main

import (
	"log"
	"net/http"
	"os"

	"github.com/getopswise/opswise/app/internal/api"
	"github.com/getopswise/opswise/app/internal/db"
	"github.com/getopswise/opswise/app/internal/db/dbq"
	"github.com/getopswise/opswise/app/web/templates"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

func main() {
	dbPath := os.Getenv("OPSWISE_DB")
	if dbPath == "" {
		dbPath = "opswise.db"
	}

	database, err := db.Open(dbPath)
	if err != nil {
		log.Fatalf("failed to open database: %v", err)
	}
	defer database.Close()

	queries := dbq.New(database)

	hostHandler := api.NewHostHandler(queries)
	productHandler := api.NewProductHandler(queries)
	stackHandler := api.NewStackHandler(queries)
	deploymentHandler := api.NewDeploymentHandler(queries)
	settingsHandler := api.NewSettingsHandler(queries)

	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	// Static files
	fs := http.FileServer(http.Dir("web/static"))
	r.Handle("/static/*", http.StripPrefix("/static/", fs))

	// Dashboard
	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		templates.Dashboard().Render(r.Context(), w)
	})

	// Hosts
	r.Get("/hosts", hostHandler.List)
	r.Post("/hosts", hostHandler.Create)
	r.Delete("/hosts/{id}", hostHandler.Delete)

	// Products
	r.Get("/products", productHandler.List)

	// Stacks
	r.Get("/stacks", stackHandler.List)

	// Deployments
	r.Get("/deployments", deploymentHandler.List)

	// Settings
	r.Get("/settings", settingsHandler.Page)

	addr := ":8080"
	log.Printf("Opswise Deploy starting on %s", addr)
	if err := http.ListenAndServe(addr, r); err != nil {
		log.Fatalf("server error: %v", err)
	}
}
