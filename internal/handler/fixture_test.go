package handler

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"optipay/internal/middleware"
	"optipay/internal/repository"
	"optipay/internal/service"

	"github.com/DATA-DOG/go-sqlmock"
)

// handlerFixture agrupa todos los servicios y repos construidos sobre una única
// conexión sqlmock, tal como los monta buildRouter en cmd/server. Cada test
// usa mock para registrar las consultas que cada llamada al servicio dispara.
type handlerFixture struct {
	db   *sql.DB
	mock sqlmock.Sqlmock

	authSvc  *service.AuthService
	transSvc *service.TransaccionService
	cfSvc    *service.CostoFijoService
	mesSvc   *service.MesService
	deudaSvc *service.DeudaService
	dashSvc  *service.DashboardService
	catRepo  *repository.CategoriaRepo

	authH  *AuthHandler
	transH *TransaccionHandler
	cfH    *CostoFijoHandler
	mesH   *MesHandler
	deudaH *DeudaHandler
	dashH  *DashboardHandler
	catH   *CategoriaHandler
	pagesH *PagesHandler
}

// newHandlerFixture construye la cadena handler->service->repository->sqlmock.
func newHandlerFixture(t *testing.T) *handlerFixture {
	t.Helper()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	ur := repository.NewUsuarioRepo(db)
	mr := repository.NewMesRepo(db)
	tr := repository.NewTransaccionRepo(db)
	cfr := repository.NewCostoFijoRepo(db)
	dr := repository.NewDeudaRepo(db)
	cr := repository.NewCategoriaRepo(db)

	mailer := &fakeMailer{}
	authSvc := service.NewAuthService(ur, []byte("test-secret"), 72*time.Hour, mailer, "http://localhost:8080")
	transSvc := service.NewTransaccionService(tr, mr)
	cfSvc := service.NewCostoFijoService(cfr, mr)
	mesSvc := service.NewMesService(mr, tr, cfr, dr)
	deudaSvc := service.NewDeudaService(dr, cr, transSvc)
	dashSvc := service.NewDashboardService(mr, tr, cr, dr)

	return &handlerFixture{
		db:       db,
		mock:     mock,
		authSvc:  authSvc,
		transSvc: transSvc,
		cfSvc:    cfSvc,
		mesSvc:   mesSvc,
		deudaSvc: deudaSvc,
		dashSvc:  dashSvc,
		catRepo:  cr,
		authH:    NewAuthHandler(authSvc),
		transH:   NewTransaccionHandler(transSvc),
		cfH:      NewCostoFijoHandler(cfSvc),
		mesH:     NewMesHandler(mesSvc),
		deudaH:   NewDeudaHandler(deudaSvc),
		dashH:    NewDashboardHandler(dashSvc),
		catH:     NewCategoriaHandler(cr),
		pagesH:   NewPagesHandler(dashSvc, transSvc, cfSvc, mesSvc, deudaSvc, cr, authSvc),
	}
}

// ctxWithUserID devuelve un contexto con el userID inyectado (equivale al paso
// por el middleware JWTAuth).
func ctxWithUserID(userID int64) context.Context {
	return context.WithValue(context.Background(), middleware.UserIDKey, userID)
}

// fakeMailer es un doble del Mailer que no envía correos reales.
type fakeMailer struct{}

func (f *fakeMailer) SendVerificacion(context.Context, string, string, string) error  { return nil }
func (f *fakeMailer) SendPasswordReset(context.Context, string, string, string) error { return nil }
