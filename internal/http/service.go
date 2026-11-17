package http

import (
	api "github.com/makkssik/template/internal/generated"
	"github.com/makkssik/template/internal/http/health"
	positioncreate "github.com/makkssik/template/internal/http/position/create"
	positionlist "github.com/makkssik/template/internal/http/position/list"
	"github.com/makkssik/template/internal/http/ready"
	"github.com/makkssik/template/internal/http/trip/create"
	"github.com/makkssik/template/internal/http/trip/finish"
	"github.com/makkssik/template/internal/http/trip/get"
)

type Implementation struct {
	api.Unimplemented
	positionCreate *positioncreate.Handler
	positionList   *positionlist.Handler
	createHandler  *create.Handler
	getHandler     *get.Handler
	finishHandler  *finish.Handler
	healthHandler  *health.Handler
	readyHandler   *ready.Handler
}

type ImplementationCfg struct {
	PositionCreate *positioncreate.Handler
	PositionList   *positionlist.Handler
	CreateHandler  *create.Handler
	GetHandler     *get.Handler
	FinishHandler  *finish.Handler
	HealthHandler  *health.Handler
	ReadyHandler   *ready.Handler
}

func New(cfg ImplementationCfg) *Implementation {
	return &Implementation{
		positionCreate: cfg.PositionCreate,
		positionList:   cfg.PositionList,
		createHandler:  cfg.CreateHandler,
		getHandler:     cfg.GetHandler,
		finishHandler:  cfg.FinishHandler,
		healthHandler:  cfg.HealthHandler,
		readyHandler:   cfg.ReadyHandler,
	}
}
