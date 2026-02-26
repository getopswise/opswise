package api

import (
	"net/http"

	"github.com/getopswise/opswise/app/internal/db/dbq"
	"github.com/getopswise/opswise/app/web/templates"
)

type ProductHandler struct {
	q *dbq.Queries
}

func NewProductHandler(q *dbq.Queries) *ProductHandler {
	return &ProductHandler{q: q}
}

func (h *ProductHandler) List(w http.ResponseWriter, r *http.Request) {
	products, err := h.q.ListProducts(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	templates.ProductsPage(products).Render(r.Context(), w)
}
