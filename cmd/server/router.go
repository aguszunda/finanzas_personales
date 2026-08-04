package main

import (
	"database/sql"
	"net/http"
	"time"

	"finanzas_personales/internal/config"
	"finanzas_personales/internal/handler"
	"finanzas_personales/internal/middleware"
	"finanzas_personales/internal/repository"
	"finanzas_personales/internal/service"
	"finanzas_personales/web"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
)

// buildRouter wires the whole application: repositories, services, handlers,
// middleware and routes. It is shared between main() and the integration tests
// so both exercise the exact same stack.
func buildRouter(cfg *config.Config, db *sql.DB) http.Handler {
	tmpl := handler.NewTemplateManager(web.TemplateFS())
	handler.SetTemplateManager(tmpl)

	usuarioRepo := repository.NewUsuarioRepo(db)
	categoriaRepo := repository.NewCategoriaRepo(db)
	transaccionRepo := repository.NewTransaccionRepo(db)
	costoFijoRepo := repository.NewCostoFijoRepo(db)
	mesRepo := repository.NewMesRepo(db)
	deudaRepo := repository.NewDeudaRepo(db)

	authSvc := service.NewAuthService(usuarioRepo, []byte(cfg.JWTSecret), cfg.JWTExpiration)
	transSvc := service.NewTransaccionService(transaccionRepo, mesRepo)
	cfSvc := service.NewCostoFijoService(costoFijoRepo, mesRepo)
	mesSvc := service.NewMesService(mesRepo, transaccionRepo, costoFijoRepo, deudaRepo)
	deudaSvc := service.NewDeudaService(deudaRepo)
	dashSvc := service.NewDashboardService(mesRepo, transaccionRepo, categoriaRepo)

	authH := handler.NewAuthHandler(authSvc)
	transH := handler.NewTransaccionHandler(transSvc)
	cfH := handler.NewCostoFijoHandler(cfSvc)
	mesH := handler.NewMesHandler(mesSvc)
	deudaH := handler.NewDeudaHandler(deudaSvc)
	dashH := handler.NewDashboardHandler(dashSvc)
	catH := handler.NewCategoriaHandler(categoriaRepo)
	pagesH := handler.NewPagesHandler(dashSvc, transSvc, cfSvc, mesSvc, deudaSvc, categoriaRepo)

	r := chi.NewRouter()

	r.Use(chimw.RequestID)
	r.Use(chimw.Recoverer)
	r.Use(middleware.Logging)
	r.Use(chimw.Timeout(30 * time.Second))
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{cfg.CORSOrigin},
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Content-Type", "Authorization", "HX-Request"},
		AllowCredentials: true,
	}))
	r.Use(middleware.DetectHTMX)

	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})

	r.Get("/login", pagesH.LoginPage)
	r.Get("/register", pagesH.RegisterPage)
	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		if _, err := r.Cookie("token"); err == nil {
			http.Redirect(w, r, "/api/dashboard/page", http.StatusFound)
			return
		}
		http.Redirect(w, r, "/login", http.StatusFound)
	})

	r.Route("/api", func(r chi.Router) {
		r.Post("/auth/register", authH.Register)
		r.Post("/auth/login", authH.Login)

		r.Group(func(r chi.Router) {
			r.Use(middleware.JWTAuth([]byte(cfg.JWTSecret)))

			r.Route("/transacciones", func(r chi.Router) {
				r.Get("/", transH.List)
				r.Post("/", transH.Create)
				r.Get("/{id}", transH.GetByID)
				r.Put("/{id}", transH.Update)
				r.Delete("/{id}", transH.Delete)
			})

			r.Route("/costos-fijos", func(r chi.Router) {
				r.Get("/", cfH.List)
				r.Post("/", cfH.Create)
				r.Get("/{id}", cfH.GetByID)
				r.Put("/{id}", cfH.Update)
				r.Patch("/{id}/toggle", cfH.Toggle)
				r.Delete("/{id}", cfH.Delete)
			})

			r.Route("/meses", func(r chi.Router) {
				r.Get("/", mesH.List)
				r.Get("/current", mesH.Current)
				r.Get("/{id}", mesH.GetByID)
				r.Post("/{id}/cerrar", mesH.Cerrar)
				r.Post("/{id}/recalcular", mesH.Recalcular)
			})

			r.Route("/deudas", func(r chi.Router) {
				r.Get("/", deudaH.List)
				r.Post("/", deudaH.Create)
				r.Get("/{id}", deudaH.GetByID)
				r.Put("/{id}", deudaH.Update)
				r.Delete("/{id}", deudaH.Delete)
			})

			r.Get("/dashboard", dashH.GetDashboard)
			r.Get("/categorias", catH.List)

			r.Get("/dashboard/page", pagesH.DashboardPage)
			r.Get("/transacciones/page", pagesH.TransaccionesPage)
			r.Get("/costos-fijos/page", pagesH.CostosFijosPage)
			r.Get("/balance/page", pagesH.BalancePage)
			r.Get("/balance/{id}/page", pagesH.BalancePage)
			r.Get("/meses/page", pagesH.MesesPage)
			r.Get("/deudas/page", pagesH.DeudasPage)
		})
	})

	return r
}
