package http

import (
	api "github.com/makkssik/template/internal/generated"
	"github.com/makkssik/template/internal/http/health"
	"github.com/makkssik/template/internal/http/ready"
	"github.com/makkssik/template/internal/http/trip/create"
	"github.com/makkssik/template/internal/http/trip/finish"
	"github.com/makkssik/template/internal/http/trip/get"
)

type Implementation struct {
	api.Unimplemented
	createHandler *create.Handler
	getHandler    *get.Handler
	finishHandler *finish.Handler
	healthHandler *health.Handler
	readyHandler  *ready.Handler
}

func New(createHandler *create.Handler, getHandler *get.Handler, finishHandler *finish.Handler, healthHandler *health.Handler, readyHandler *ready.Handler) *Implementation {
	return &Implementation{createHandler: createHandler, getHandler: getHandler, finishHandler: finishHandler, healthHandler: healthHandler, readyHandler: readyHandler}
}
