package db

import (
	"context"
	"database/sql"
	"log"

	"github.com/getopswise/opswise/app/internal/db/dbq"
)

func Seed(ctx context.Context, q *dbq.Queries) {
	seedProducts(ctx, q)
	seedStacks(ctx, q)
}

func seedProducts(ctx context.Context, q *dbq.Queries) {
	products := []dbq.UpsertProductParams{
		{
			Name:        "grafana",
			DisplayName: "Grafana",
			Description: ns("Open source analytics and monitoring platform"),
			Version:     ns("10.4.0"),
			Icon:        ns("chart-bar"),
			Modes:       `["ansible","compose","helm"]`,
		},
		{
			Name:        "prometheus",
			DisplayName: "Prometheus",
			Description: ns("Open source monitoring and alerting toolkit"),
			Version:     ns("2.51.0"),
			Icon:        ns("fire"),
			Modes:       `["ansible","compose","helm"]`,
		},
		{
			Name:        "loki",
			DisplayName: "Loki",
			Description: ns("Horizontally scalable log aggregation system by Grafana"),
			Version:     ns("2.9.6"),
			Icon:        ns("file-text"),
			Modes:       `["ansible","compose"]`,
		},
		{
			Name:        "gitlab",
			DisplayName: "GitLab CE",
			Description: ns("Complete DevOps platform with git repository management, CI/CD, and more"),
			Version:     ns("16.11.0"),
			Icon:        ns("git-branch"),
			Modes:       `["ansible","compose"]`,
		},
		{
			Name:        "harbor",
			DisplayName: "Harbor",
			Description: ns("Open source container image registry with security and access controls"),
			Version:     ns("2.10.0"),
			Icon:        ns("package"),
			Modes:       `["ansible","compose","helm"]`,
		},
		{
			Name:        "keycloak",
			DisplayName: "Keycloak",
			Description: ns("Open source identity and access management"),
			Version:     ns("24.0.0"),
			Icon:        ns("shield"),
			Modes:       `["ansible","compose","helm"]`,
		},
		{
			Name:        "argocd",
			DisplayName: "ArgoCD",
			Description: ns("Declarative GitOps continuous delivery tool for Kubernetes"),
			Version:     ns("2.11.0"),
			Icon:        ns("refresh-cw"),
			Modes:       `["helm"]`,
		},
	}

	for _, p := range products {
		if err := q.UpsertProduct(ctx, p); err != nil {
			log.Printf("seed product %s: %v", p.Name, err)
		}
	}
}

func seedStacks(ctx context.Context, q *dbq.Queries) {
	stacks := []dbq.UpsertStackParams{
		{
			Name:        "monitoring",
			DisplayName: "Monitoring Stack",
			Description: ns("Prometheus + Grafana + Loki for full observability"),
			Products:    `["prometheus","grafana","loki"]`,
		},
		{
			Name:        "cicd",
			DisplayName: "CI/CD Stack",
			Description: ns("GitLab CE + Harbor for source control and container registry"),
			Products:    `["gitlab","harbor"]`,
		},
		{
			Name:        "vibe-coding",
			DisplayName: "Vibe Coding Stack",
			Description: ns("GitLab + Harbor + Keycloak + ArgoCD for a full development platform"),
			Products:    `["gitlab","harbor","keycloak","argocd"]`,
		},
	}

	for _, s := range stacks {
		if err := q.UpsertStack(ctx, s); err != nil {
			log.Printf("seed stack %s: %v", s.Name, err)
		}
	}
}

func ns(s string) sql.NullString {
	return sql.NullString{String: s, Valid: true}
}
