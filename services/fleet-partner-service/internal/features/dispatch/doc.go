// Package dispatch implements Section 16 corporate dispatch planning,
// explainable candidates, control-tower projections, geofences, operational
// alerts and rescue cases. It owns planning and projections only: canonical
// execution and authoritative provider location remain in Dispatch/Hauling.
// Route planning lives in features/routes and tracking projections in
// features/tracking; dispatch reaches them through ports wired in
// internal/app.
//
// Layout: models (domain, pure rules and the shared Section 16 kernel),
// repositories (SQL), usecases (workflows, workers, maintenance) and http.
package dispatch
