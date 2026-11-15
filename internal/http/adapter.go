package http

import (
	"net/http"

	api "github.com/makkssik/template/internal/generated"
)

func (i *Implementation) GetTrip(w http.ResponseWriter, r *http.Request, id api.TripId) {
	i.getHandler.GetTrip(w, r, id)
}

func (i *Implementation) FinishTrip(w http.ResponseWriter, r *http.Request, id api.TripId) {
	i.finishHandler.FinishTrip(w, r, id)
}

func (i *Implementation) CreateTrip(w http.ResponseWriter, r *http.Request, params api.CreateTripParams) {
	i.createHandler.CreateTrip(w, r, params)
}

func (i *Implementation) Health(w http.ResponseWriter, r *http.Request) {
	i.healthHandler.Health(w, r)
}

func (i *Implementation) Ready(w http.ResponseWriter, r *http.Request) {
	i.readyHandler.Ready(w, r)
}
