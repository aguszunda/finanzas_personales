package model

import "time"

type Usuario struct {
	ID            int64     `json:"id"`
	Nombre        string    `json:"nombre"`
	Email         string    `json:"email"`
	PasswordHash  string    `json:"-"`
	MonedaDefault string    `json:"moneda_default"`
	CreatedAt     time.Time `json:"created_at"`
}

type Transaccion struct {
	ID          int64     `json:"id"`
	UsuarioID   int64     `json:"usuario_id"`
	Tipo        string    `json:"tipo"`
	Monto       float64   `json:"monto"`
	Fecha       string    `json:"fecha"`
	CategoriaID int64     `json:"categoria_id"`
	Categoria   string    `json:"categoria_nombre,omitempty"`
	Descripcion string    `json:"descripcion"`
	MedioPago   string    `json:"medio_pago"`
	EsFijo      bool      `json:"es_fijo"`
	CuotasTotal *int      `json:"cuotas_total,omitempty"`
	CuotaActual *int      `json:"cuota_actual,omitempty"`
	Estado      string    `json:"estado"`
	MesID       *int64    `json:"mes_id,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type CostoFijo struct {
	ID             int64     `json:"id"`
	UsuarioID      int64     `json:"usuario_id"`
	CategoriaID    int64     `json:"categoria_id"`
	Categoria      string    `json:"categoria_nombre,omitempty"`
	Descripcion    string    `json:"descripcion"`
	MontoEstimado  float64   `json:"monto_estimado"`
	DiaVencimiento int       `json:"dia_vencimiento"`
	Activo         bool      `json:"activo"`
	TipoPeriodo    string    `json:"tipo_periodo"`
	CreatedAt      time.Time `json:"created_at"`
}

type Categoria struct {
	ID              int64     `json:"id"`
	Nombre          string    `json:"nombre"`
	Tipo            string    `json:"tipo"`
	Icono           string    `json:"icono"`
	EsPersonalizada bool      `json:"es_personalizada"`
	UsuarioID       *int64    `json:"usuario_id,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
}

type Mes struct {
	ID              int64     `json:"id"`
	UsuarioID       int64     `json:"usuario_id"`
	Periodo         string    `json:"periodo"`
	Estado          string    `json:"estado"`
	IngresosTotal   float64   `json:"ingresos_total"`
	EgresosTotal    float64   `json:"egresos_total"`
	Superavit       float64   `json:"superavit"`
	TasaAhorro      *float64  `json:"tasa_ahorro,omitempty"`
	AhorroAcumulado float64   `json:"ahorro_acumulado"`
	PasivosTotal    float64   `json:"pasivos_total"`
	Patrimonio      float64   `json:"patrimonio"`
	CreatedAt       time.Time `json:"created_at"`
}

type MetaAhorro struct {
	ID            int64     `json:"id"`
	UsuarioID     int64     `json:"usuario_id"`
	Nombre        string    `json:"nombre"`
	MontoObjetivo float64   `json:"monto_objetivo"`
	MontoActual   float64   `json:"monto_actual"`
	FechaLimite   string    `json:"fecha_limite"`
	CreatedAt     time.Time `json:"created_at"`
}

type Presupuesto struct {
	ID          int64     `json:"id"`
	UsuarioID   int64     `json:"usuario_id"`
	CategoriaID int64     `json:"categoria_id"`
	MesID       int64     `json:"mes_id"`
	MontoLimite float64   `json:"monto_limite"`
	CreatedAt   time.Time `json:"created_at"`
}

type Deuda struct {
	ID              int64     `json:"id"`
	UsuarioID       int64     `json:"usuario_id"`
	Tipo            string    `json:"tipo"`
	Entidad         string    `json:"entidad"`
	MontoTotal      float64   `json:"monto_total"`
	CuotasRestantes int       `json:"cuotas_restantes"`
	TasaInteres     float64   `json:"tasa_interes"`
	FechaCierre     string    `json:"fecha_cierre"`
	FechaPago       string    `json:"fecha_pago"`
	CreatedAt       time.Time `json:"created_at"`
}

type Inversion struct {
	ID              int64     `json:"id"`
	UsuarioID       int64     `json:"usuario_id"`
	TipoInstrumento string    `json:"tipo_instrumento"`
	MontoInvertido  float64   `json:"monto_invertido"`
	ValorActual     float64   `json:"valor_actual"`
	Rentabilidad    float64   `json:"rentabilidad"`
	CreatedAt       time.Time `json:"created_at"`
}
